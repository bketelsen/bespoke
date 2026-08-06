package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bketelsen/bespoke/internal/manifest"
	"github.com/bketelsen/bespoke/pkg/auth"
	bespokeevents "github.com/bketelsen/bespoke/pkg/events"
	"github.com/google/uuid"
)

var eventTypeRE = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
var eventSourceRE = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
var errEventDataLarge = errors.New("event data too large")

type eventService struct {
	db           *sql.DB
	root, domain string
	dev          bool
	mu           sync.Mutex
	subs         map[string]map[chan bespokeevents.Notification]struct{}
	workerID     string
	stop         chan struct{}
}

func newEventService(db *sql.DB, root, domain string, dev bool) *eventService {
	s := &eventService{db: db, root: root, domain: domain, dev: dev, subs: map[string]map[chan bespokeevents.Notification]struct{}{}, workerID: uuid.NewString(), stop: make(chan struct{})}
	go s.worker()
	return s
}

type publishRequest struct {
	Source      string              `json:"source"`
	User        string              `json:"user"`
	CausationID string              `json:"causation_id,omitempty"`
	Event       bespokeevents.Event `json:"event"`
}

func (s *eventService) registerInternal(mux *http.ServeMux) {
	mux.HandleFunc("GET /events/healthz", s.logged("health", func(w http.ResponseWriter, r *http.Request) {
		if err := s.db.PingContext(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	}))
	mux.HandleFunc("POST /events/publish", s.logged("publish", s.publish))
	mux.HandleFunc("GET /events/notifications", s.logged("notifications-list", s.internalList))
	mux.HandleFunc("GET /events/notifications/unread", s.logged("notifications-unread", s.internalUnread))
	mux.HandleFunc("GET /events/notifications/live", s.logged("notifications-live", s.internalLive))
	mux.HandleFunc("POST /events/notifications/read-all", s.logged("notifications-read-all", s.internalReadAll))
	mux.HandleFunc("POST /events/notifications/{id}/{action}", s.logged("notifications-mutate", s.internalMutate))
}

func (s *eventService) logged(op string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &eventStatusWriter{ResponseWriter: w, status: 200}
		next(sw, r)
		log.Printf("events op=%s source=%s user=%s status=%d dur=%s", op, r.URL.Query().Get("source"), r.URL.Query().Get("user"), sw.status, time.Since(start).Round(time.Millisecond))
	}
}

type eventStatusWriter struct {
	http.ResponseWriter
	status int
}

func (w *eventStatusWriter) WriteHeader(n int) { w.status = n; w.ResponseWriter.WriteHeader(n) }
func (w *eventStatusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (w *eventStatusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func validatePublish(p publishRequest) error {
	if _, err := uuid.Parse(p.Event.ID); err != nil {
		return fmt.Errorf("event id must be UUID")
	}
	if !eventSourceRE.MatchString(p.Source) {
		return fmt.Errorf("invalid source")
	}
	if p.User == "" || len(p.User) > 320 {
		return fmt.Errorf("invalid user")
	}
	if !eventTypeRE.MatchString(p.Event.Type) || len(p.Event.Type) > 100 {
		return fmt.Errorf("invalid event type")
	}
	if len(p.Event.SubjectID) > 512 || p.Event.OccurredAt.IsZero() {
		return fmt.Errorf("invalid subject or occurred_at")
	}
	if p.Event.Data == nil {
		return fmt.Errorf("event data must be an object")
	}
	b, err := json.Marshal(p.Event.Data)
	if len(b) > 64<<10 {
		return errEventDataLarge
	}
	if err != nil || jsonDepth(p.Event.Data) > 8 {
		return fmt.Errorf("invalid event data")
	}
	if n := p.Event.Notification; n != nil {
		if n.Title == "" || len(n.Title) > 120 || len(n.Body) > 500 || len(n.GroupKey) > 200 || len(n.Path) > 1024 || (n.Path != "" && !strings.HasPrefix(n.Path, "/")) {
			return fmt.Errorf("invalid notification")
		}
	}
	return nil
}
func jsonDepth(v any) int {
	switch x := v.(type) {
	case map[string]any:
		d := 1
		for _, v := range x {
			if n := 1 + jsonDepth(v); n > d {
				d = n
			}
		}
		return d
	case []any:
		d := 1
		for _, v := range x {
			if n := 1 + jsonDepth(v); n > d {
				d = n
			}
		}
		return d
	}
	return 0
}

func (s *eventService) publish(w http.ResponseWriter, r *http.Request) {
	var p publishRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 80<<10))
	dec.UseNumber()
	if err := dec.Decode(&p); err != nil {
		if strings.Contains(err.Error(), "too large") {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "bad request", 400)
		}
		return
	}
	if err := validatePublish(p); err != nil {
		if errors.Is(err, errEventDataLarge) {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, err.Error(), 400)
		}
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(r.Context(), "SELECT count(*) FROM events WHERE source=? AND id=?", p.Source, p.Event.ID).Scan(&exists); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if exists > 0 {
		jsonResponse(w, 200, map[string]string{"id": p.Event.ID})
		return
	}
	depth, correlation := 0, p.Event.ID
	if p.CausationID != "" {
		var parentDepth int
		var parentUser, parentCorrelation string
		err = tx.QueryRowContext(r.Context(), "SELECT login,correlation_id,depth FROM events WHERE id=?", p.CausationID).Scan(&parentUser, &parentCorrelation, &parentDepth)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && parentUser != p.User) {
			http.Error(w, "invalid causation_id", 400)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if parentDepth >= 8 {
			http.Error(w, "causation depth exceeded", http.StatusConflict)
			return
		}
		depth, correlation = parentDepth+1, parentCorrelation
	}
	data, _ := json.Marshal(p.Event.Data)
	_, err = tx.ExecContext(r.Context(), `INSERT INTO events(source,id,login,type,subject_id,occurred_at,data_json,causation_id,correlation_id,depth) VALUES(?,?,?,?,?,?,?,?,?,?)`, p.Source, p.Event.ID, p.User, p.Event.Type, p.Event.SubjectID, p.Event.OccurredAt.UTC().Format(time.RFC3339Nano), string(data), nullString(p.CausationID), correlation, depth)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var created *bespokeevents.Notification
	if p.Event.Notification != nil {
		n, inserted, err := s.insertNotification(r.Context(), tx, p.User, p.Source, p.Event.ID, *p.Event.Notification, "")
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if inserted {
			created = &n
		}
	}
	if depth < 8 {
		if err = s.enqueueMatching(r.Context(), tx, p); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if created != nil {
		s.emit(p.User, *created)
	}
	jsonResponse(w, 202, map[string]string{"id": p.Event.ID})
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func (s *eventService) resolveURL(appSlug, path string) (string, error) {
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("path must begin with /")
	}
	if appSlug == "" {
		if s.dev {
			return "http://localhost:4000" + path, nil
		}
		return "https://" + s.domain + path, nil
	}
	a, err := manifest.Load(s.root, appSlug)
	if err != nil {
		return "", fmt.Errorf("unknown app %s", appSlug)
	}
	if s.dev {
		return fmt.Sprintf("http://localhost:%d%s", a.Port, path), nil
	}
	return fmt.Sprintf("https://%s.%s%s", appSlug, s.domain, path), nil
}

func (s *eventService) insertNotification(ctx context.Context, tx *sql.Tx, login, source, eventID string, n bespokeevents.Notification, dedup string) (bespokeevents.Notification, bool, error) {
	resolved, err := s.resolveURL(n.AppSlug, n.Path)
	if err != nil {
		return n, false, err
	}
	n.ID = uuid.NewString()
	n.EventID = eventID
	n.Source = source
	n.URL = resolved
	n.DedupKey = dedup
	n.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO notifications(id,event_id,source,login,title,body,app_slug,path,group_key,dedup_key,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, n.ID, eventID, source, login, n.Title, n.Body, n.AppSlug, n.Path, n.GroupKey, nullString(dedup), n.CreatedAt)
	if err != nil {
		return n, false, err
	}
	inserted, _ := res.RowsAffected()
	return n, inserted == 1, nil
}

func (s *eventService) subscribe(login string) (chan bespokeevents.Notification, func()) {
	ch := make(chan bespokeevents.Notification, 8)
	s.mu.Lock()
	if s.subs[login] == nil {
		s.subs[login] = map[chan bespokeevents.Notification]struct{}{}
	}
	s.subs[login][ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() { s.mu.Lock(); delete(s.subs[login], ch); s.mu.Unlock() }
}
func (s *eventService) emit(login string, n bespokeevents.Notification) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs[login] {
		select {
		case ch <- n:
		default:
		}
	}
}

func (s *eventService) internalList(w http.ResponseWriter, r *http.Request) {
	login := r.URL.Query().Get("user")
	if login == "" {
		http.Error(w, "user required", 400)
		return
	}
	limit := 30
	if n, _ := strconv.Atoi(r.URL.Query().Get("limit")); n > 0 {
		limit = n
	}
	if limit > 100 {
		limit = 100
	}
	args := []any{login}
	where := "login=? AND dismissed_at IS NULL"
	if cur := r.URL.Query().Get("after"); cur != "" {
		b, e := base64.RawURLEncoding.DecodeString(cur)
		parts := strings.SplitN(string(b), "|", 2)
		if e != nil || len(parts) != 2 {
			http.Error(w, "invalid cursor", 400)
			return
		}
		where += " AND (created_at < ? OR (created_at = ? AND id < ?))"
		args = append(args, parts[0], parts[0], parts[1])
	}
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,event_id,source,title,body,app_slug,path,group_key,COALESCE(dedup_key,''),created_at,read_at,dismissed_at FROM notifications WHERE `+where+` ORDER BY created_at DESC,id DESC LIMIT ?`, args...)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	page := bespokeevents.Page{}
	for rows.Next() {
		var n bespokeevents.Notification
		var read, dismiss sql.NullString
		if err := rows.Scan(&n.ID, &n.EventID, &n.Source, &n.Title, &n.Body, &n.AppSlug, &n.Path, &n.GroupKey, &n.DedupKey, &n.CreatedAt, &read, &dismiss); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if read.Valid {
			n.ReadAt = &read.String
		}
		if dismiss.Valid {
			n.DismissedAt = &dismiss.String
		}
		var resolveErr error
		n.URL, resolveErr = s.resolveURL(n.AppSlug, n.Path)
		n.Retired = resolveErr != nil
		page.Notifications = append(page.Notifications, n)
	}
	if len(page.Notifications) > limit {
		last := page.Notifications[limit-1]
		page.Next = base64.RawURLEncoding.EncodeToString([]byte(last.CreatedAt + "|" + last.ID))
		page.Notifications = page.Notifications[:limit]
	}
	jsonResponse(w, 200, page)
}
func (s *eventService) internalUnread(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("user") == "" {
		http.Error(w, "user required", 400)
		return
	}
	var n int
	err := s.db.QueryRowContext(r.Context(), "SELECT count(*) FROM notifications WHERE login=? AND read_at IS NULL AND dismissed_at IS NULL", r.URL.Query().Get("user")).Scan(&n)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if n > 999 {
		n = 999
	}
	jsonResponse(w, 200, map[string]int{"count": n})
}
func (s *eventService) internalMutate(w http.ResponseWriter, r *http.Request) {
	var b struct {
		User string `json:"user"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&b) != nil || b.User == "" {
		http.Error(w, "bad request", 400)
		return
	}
	col := map[string]string{"read": "read_at", "dismiss": "dismissed_at"}[r.PathValue("action")]
	if col == "" {
		http.NotFound(w, r)
		return
	}
	res, err := s.db.ExecContext(r.Context(), "UPDATE notifications SET "+col+"=COALESCE("+col+",datetime('now')) WHERE id=? AND login=?", r.PathValue("id"), b.User)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(204)
}
func (s *eventService) internalReadAll(w http.ResponseWriter, r *http.Request) {
	var b struct {
		User string `json:"user"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&b) != nil || b.User == "" {
		http.Error(w, "bad request", 400)
		return
	}
	_, err := s.db.ExecContext(r.Context(), "UPDATE notifications SET read_at=COALESCE(read_at,datetime('now')) WHERE login=? AND dismissed_at IS NULL", b.User)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}
func (s *eventService) internalLive(w http.ResponseWriter, r *http.Request) {
	login := r.URL.Query().Get("user")
	if login == "" {
		http.Error(w, "user required", 400)
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	ch, cancel := s.subscribe(login)
	defer cancel()
	tick := time.NewTicker(45 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case n := <-ch:
			b, _ := json.Marshal(n)
			if _, err := fmt.Fprintf(w, "event: notification\ndata: %s\n\n", b); err != nil {
				return
			}
			f.Flush()
		case <-tick.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			f.Flush()
		}
	}
}

func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// registerApexAutomation is implemented in automation.go.
func (s *eventService) registerApex(mux *http.ServeMux) { s.registerAutomationRoutes(mux) }

func userFrom(r *http.Request) auth.User { return auth.FromContext(r.Context()) }
func parseJSONBody(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(v); err != nil {
		http.Error(w, "bad request", 400)
		return false
	}
	return true
}
