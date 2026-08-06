package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bketelsen/bespoke/internal/manifest"
	"github.com/bketelsen/bespoke/pkg/auth"
	bespokeevents "github.com/bketelsen/bespoke/pkg/events"
	"github.com/bketelsen/bespoke/pkg/llm"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
)

type condition struct {
	Path     string `json:"path"`
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
}
type automationStep struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Title       string         `json:"title,omitempty"`
	Body        string         `json:"body,omitempty"`
	AppSlug     string         `json:"app_slug,omitempty"`
	Path        string         `json:"path,omitempty"`
	GroupKey    string         `json:"group_key,omitempty"`
	Instruction string         `json:"instruction,omitempty"`
	Input       map[string]any `json:"input,omitempty"`
	Schema      map[string]any `json:"schema,omitempty"`
	ToolApp     string         `json:"tool_app,omitempty"`
	ToolName    string         `json:"tool_name,omitempty"`
	Args        map[string]any `json:"args,omitempty"`
}
type automationRule struct {
	ID         string           `json:"id,omitempty"`
	Name       string           `json:"name"`
	Enabled    bool             `json:"enabled"`
	Source     string           `json:"source"`
	EventType  string           `json:"event_type"`
	Conditions []condition      `json:"conditions"`
	Steps      []automationStep `json:"steps"`
	Revision   int              `json:"revision,omitempty"`
	EnabledAt  *string          `json:"enabled_at"`
	CreatedAt  string           `json:"created_at,omitempty"`
	UpdatedAt  string           `json:"updated_at,omitempty"`
}

func (s *eventService) validateRule(ctx context.Context, rule automationRule) error {
	if rule.Name == "" || len(rule.Name) > 100 || !eventTypeRE.MatchString(rule.EventType) || len(rule.Conditions) > 12 || len(rule.Steps) == 0 || len(rule.Steps) > 8 {
		return fmt.Errorf("invalid rule")
	}
	if _, err := manifest.Load(s.root, rule.Source); err != nil {
		return fmt.Errorf("unknown source")
	}
	seen := map[string]bool{}
	for _, c := range rule.Conditions {
		if c.Path == "" || !validOperator(c.Operator) {
			return fmt.Errorf("invalid condition")
		}
	}
	tools := allAppTools(ctx, s.root)
	for _, st := range rule.Steps {
		if st.Name == "" || seen[st.Name] {
			return fmt.Errorf("invalid step name")
		}
		seen[st.Name] = true
		switch st.Type {
		case "notify":
			if st.Title == "" || len(st.Title) > 120 {
				return fmt.Errorf("invalid notify step")
			}
		case "ai_json":
			b, _ := json.Marshal(st.Schema)
			if st.Instruction == "" || len(st.Instruction) > 4<<10 || len(b) > 16<<10 {
				return fmt.Errorf("invalid ai_json step")
			}
			var sch jsonschema.Schema
			if json.Unmarshal(b, &sch) != nil {
				return fmt.Errorf("invalid schema")
			}
			if _, err := sch.Resolve(nil); err != nil {
				return fmt.Errorf("invalid schema: %w", err)
			}
		case "tool":
			found := false
			for _, t := range tools {
				if t.Slug == st.ToolApp && t.Name == st.ToolName && t.Automation != "" && t.Automation != "forbidden" {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("tool is not automation eligible")
			}
		default:
			return fmt.Errorf("unknown step type")
		}
	}
	return nil
}
func validOperator(op string) bool {
	switch op {
	case "equals", "not_equals", "contains", "starts_with", "exists", "greater_than", "less_than":
		return true
	}
	return false
}

func (s *eventService) enqueueMatching(ctx context.Context, tx *sql.Tx, p publishRequest) error {
	rows, err := tx.QueryContext(ctx, `SELECT id,name,source,event_type,conditions_json,steps_json,revision,enabled_at FROM automation_rules WHERE login=? AND enabled=1 AND deleted_at IS NULL AND source=? AND event_type=?`, p.User, p.Source, p.Event.Type)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r automationRule
		var conds, steps, enabledAt string
		if err := rows.Scan(&r.ID, &r.Name, &r.Source, &r.EventType, &conds, &steps, &r.Revision, &enabledAt); err != nil {
			return err
		}
		_ = json.Unmarshal([]byte(conds), &r.Conditions)
		_ = json.Unmarshal([]byte(steps), &r.Steps)
		if !matches(r.Conditions, p.Event) {
			continue
		}
		rj, _ := json.Marshal(r)
		runID := uuid.NewString()
		res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO automation_runs(id,login,rule_id,event_id,event_source,rule_revision,rule_json,status) VALUES(?,?,?,?,?,?,?,'pending')`, runID, p.User, r.ID, p.Event.ID, p.Source, r.Revision, string(rj))
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			continue
		}
		for i, st := range r.Steps {
			b, _ := json.Marshal(st)
			_, err = tx.ExecContext(ctx, `INSERT INTO automation_steps(id,run_id,name,position,type,definition_json) VALUES(?,?,?,?,?,?)`, uuid.NewString(), runID, st.Name, i, st.Type, string(b))
			if err != nil {
				return err
			}
		}
	}
	return rows.Err()
}

func matches(cs []condition, e bespokeevents.Event) bool {
	for _, c := range cs {
		v, ok := eventValue(e, c.Path)
		if !compare(v, ok, c.Operator, c.Value) {
			return false
		}
	}
	return true
}
func eventValue(e bespokeevents.Event, path string) (any, bool) {
	if path == "subject_id" {
		return e.SubjectID, e.SubjectID != ""
	}
	if !strings.HasPrefix(path, "data.") {
		return nil, false
	}
	var v any = e.Data
	for _, part := range strings.Split(strings.TrimPrefix(path, "data."), ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return v, true
}
func compare(v any, ok bool, op string, w any) bool {
	if op == "exists" {
		want, yes := w.(bool)
		if !yes {
			want = true
		}
		return ok == want
	}
	if !ok {
		return false
	}
	switch op {
	case "equals":
		return fmt.Sprint(v) == fmt.Sprint(w)
	case "not_equals":
		return fmt.Sprint(v) != fmt.Sprint(w)
	case "contains":
		return strings.Contains(fmt.Sprint(v), fmt.Sprint(w))
	case "starts_with":
		return strings.HasPrefix(fmt.Sprint(v), fmt.Sprint(w))
	case "greater_than", "less_than":
		a, aok := number(v)
		b, bok := number(w)
		if !aok || !bok {
			return false
		}
		if op == "greater_than" {
			return a > b
		}
		return a < b
	}
	return false
}
func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, e := n.Float64()
		return f, e == nil
	case int:
		return float64(n), true
	}
	return 0, false
}

func (s *eventService) registerAutomationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /_automations/rules", s.listRules)
	mux.HandleFunc("POST /_automations/rules", s.createRule)
	mux.HandleFunc("GET /_automations/rules/{id}", s.getRule)
	mux.HandleFunc("PUT /_automations/rules/{id}", s.putRule)
	mux.HandleFunc("DELETE /_automations/rules/{id}", s.deleteRule)
	mux.HandleFunc("POST /_automations/rules/{id}/{action}", s.ruleAction)
	mux.HandleFunc("GET /_automations/runs", s.listRuns)
	mux.HandleFunc("GET /_automations/runs/{id}", s.getRun)
	mux.HandleFunc("POST /_automations/runs/{id}/retry", s.retryRun)
}
func (s *eventService) listRules(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	rows, err := s.db.QueryContext(r.Context(), `SELECT id,name,enabled,source,event_type,conditions_json,steps_json,revision,enabled_at,created_at,updated_at FROM automation_rules WHERE login=? AND deleted_at IS NULL ORDER BY updated_at DESC`, u.Login)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []automationRule{}
	for rows.Next() {
		rr, err := scanRule(rows)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		out = append(out, rr)
	}
	jsonResponse(w, 200, map[string]any{"rules": out})
}

type scanner interface{ Scan(...any) error }

func scanRule(row scanner) (automationRule, error) {
	var r automationRule
	var enabled int
	var conds, steps string
	var enabledAt sql.NullString
	err := row.Scan(&r.ID, &r.Name, &enabled, &r.Source, &r.EventType, &conds, &steps, &r.Revision, &enabledAt, &r.CreatedAt, &r.UpdatedAt)
	r.Enabled = enabled == 1
	if enabledAt.Valid {
		r.EnabledAt = &enabledAt.String
	}
	_ = json.Unmarshal([]byte(conds), &r.Conditions)
	_ = json.Unmarshal([]byte(steps), &r.Steps)
	return r, err
}
func (s *eventService) getRule(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	row := s.db.QueryRowContext(r.Context(), `SELECT id,name,enabled,source,event_type,conditions_json,steps_json,revision,enabled_at,created_at,updated_at FROM automation_rules WHERE id=? AND login=? AND deleted_at IS NULL`, r.PathValue("id"), u.Login)
	rr, err := scanRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	jsonResponse(w, 200, rr)
}
func (s *eventService) createRule(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var count int
	_ = s.db.QueryRowContext(r.Context(), "SELECT count(*) FROM automation_rules WHERE login=? AND deleted_at IS NULL", u.Login).Scan(&count)
	if count >= 100 {
		http.Error(w, "rule limit reached", http.StatusConflict)
		return
	}
	var rr automationRule
	if !parseJSONBody(w, r, &rr, 128<<10) {
		return
	}
	rr.Enabled = false
	if err := s.validateRule(r.Context(), rr); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	rr.ID = uuid.NewString()
	c, _ := json.Marshal(rr.Conditions)
	st, _ := json.Marshal(rr.Steps)
	_, err := s.db.ExecContext(r.Context(), `INSERT INTO automation_rules(id,login,name,source,event_type,conditions_json,steps_json) VALUES(?,?,?,?,?,?,?)`, rr.ID, u.Login, rr.Name, rr.Source, rr.EventType, string(c), string(st))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	row := s.db.QueryRowContext(r.Context(), `SELECT id,name,enabled,source,event_type,conditions_json,steps_json,revision,enabled_at,created_at,updated_at FROM automation_rules WHERE id=? AND login=?`, rr.ID, u.Login)
	created, err := scanRule(row)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	jsonResponse(w, 201, created)
}
func (s *eventService) putRule(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var rr automationRule
	if !parseJSONBody(w, r, &rr, 128<<10) {
		return
	}
	if err := s.validateRule(r.Context(), rr); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	c, _ := json.Marshal(rr.Conditions)
	st, _ := json.Marshal(rr.Steps)
	res, err := s.db.ExecContext(r.Context(), `UPDATE automation_rules SET name=?,source=?,event_type=?,conditions_json=?,steps_json=?,revision=revision+1,updated_at=datetime('now') WHERE id=? AND login=? AND revision=? AND deleted_at IS NULL`, rr.Name, rr.Source, rr.EventType, string(c), string(st), r.PathValue("id"), u.Login, rr.Revision)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var exists int
		_ = s.db.QueryRowContext(r.Context(), "SELECT count(*) FROM automation_rules WHERE id=? AND login=? AND deleted_at IS NULL", r.PathValue("id"), u.Login).Scan(&exists)
		if exists == 0 {
			http.NotFound(w, r)
		} else {
			http.Error(w, "revision conflict", http.StatusConflict)
		}
		return
	}
	s.getRule(w, r)
}
func (s *eventService) deleteRule(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	res, err := s.db.ExecContext(r.Context(), "UPDATE automation_rules SET enabled=0,enabled_at=NULL,deleted_at=datetime('now'),revision=revision+1 WHERE id=? AND login=? AND deleted_at IS NULL", r.PathValue("id"), u.Login)
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
func (s *eventService) ruleAction(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	id, action := r.PathValue("id"), r.PathValue("action")
	if action == "dry-run" {
		s.dryRun(w, r, u, id)
		return
	}
	if action != "enable" && action != "disable" {
		http.NotFound(w, r)
		return
	}
	if action == "enable" {
		row := s.db.QueryRowContext(r.Context(), `SELECT id,name,enabled,source,event_type,conditions_json,steps_json,revision,enabled_at,created_at,updated_at FROM automation_rules WHERE id=? AND login=? AND deleted_at IS NULL`, id, u.Login)
		rr, err := scanRule(row)
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if err != nil || s.validateRule(r.Context(), rr) != nil {
			http.Error(w, "rule is no longer valid", 400)
			return
		}
	}
	enabled := 0
	enabledAt := "NULL"
	if action == "enable" {
		enabled = 1
		enabledAt = "datetime('now')"
	}
	if action == "enable" {
		enabledAt = "CASE WHEN enabled=0 THEN datetime('now') ELSE enabled_at END"
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE automation_rules SET enabled=?,enabled_at=`+enabledAt+`,revision=revision+1,updated_at=datetime('now') WHERE id=? AND login=? AND deleted_at IS NULL`, enabled, id, u.Login)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.NotFound(w, r)
		return
	}
	if action == "disable" {
		_, _ = s.db.ExecContext(r.Context(), `UPDATE automation_runs SET status='failed',finished_at=datetime('now') WHERE rule_id=? AND login=? AND status='pending'`, id, u.Login)
	}
	s.getRule(w, r)
}
func (s *eventService) dryRun(w http.ResponseWriter, r *http.Request, u auth.User, id string) {
	var b struct {
		EventID string `json:"event_id"`
	}
	if !parseJSONBody(w, r, &b, 8<<10) {
		return
	}
	rrRow := s.db.QueryRowContext(r.Context(), `SELECT id,name,enabled,source,event_type,conditions_json,steps_json,revision,enabled_at,created_at,updated_at FROM automation_rules WHERE id=? AND login=? AND deleted_at IS NULL`, id, u.Login)
	rr, err := scanRule(rrRow)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	e, err := s.loadEvent(r.Context(), u.Login, rr.Source, b.EventID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	steps := make([]map[string]any, 0, len(rr.Steps))
	for _, st := range rr.Steps {
		status := "resolved"
		if st.Type == "ai_json" {
			status = "not_executed"
		}
		steps = append(steps, map[string]any{"name": st.Name, "type": st.Type, "status": status})
	}
	jsonResponse(w, 200, map[string]any{"matches": rr.EventType == e.Type && matches(rr.Conditions, e), "steps": steps})
}

func (s *eventService) loadEvent(ctx context.Context, login, source, id string) (bespokeevents.Event, error) {
	var e bespokeevents.Event
	var data, occurred string
	err := s.db.QueryRowContext(ctx, "SELECT id,type,subject_id,occurred_at,data_json FROM events WHERE login=? AND source=? AND id=?", login, source, id).Scan(&e.ID, &e.Type, &e.SubjectID, &occurred, &data)
	_ = json.Unmarshal([]byte(data), &e.Data)
	e.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurred)
	return e, err
}
func (s *eventService) listRuns(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	q := `SELECT id,rule_id,event_id,event_source,rule_revision,status,created_at,started_at,finished_at FROM automation_runs WHERE login=?`
	args := []any{u.Login}
	if id := r.URL.Query().Get("rule_id"); id != "" {
		q += " AND rule_id=?"
		args = append(args, id)
	}
	q += " ORDER BY created_at DESC LIMIT 100"
	rows, err := s.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, rid, eid, src, status, created string
		var rev int
		var started, finished sql.NullString
		_ = rows.Scan(&id, &rid, &eid, &src, &rev, &status, &created, &started, &finished)
		out = append(out, map[string]any{"id": id, "rule_id": rid, "event_id": eid, "source": src, "rule_revision": rev, "status": status, "created_at": created, "started_at": started.String, "finished_at": finished.String})
	}
	jsonResponse(w, 200, map[string]any{"runs": out, "next": ""})
}
func (s *eventService) getRun(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var run map[string]any
	var id, status, ruleID, eventID, source, created string
	var rev int
	err := s.db.QueryRowContext(r.Context(), "SELECT id,status,rule_id,event_id,event_source,rule_revision,created_at FROM automation_runs WHERE id=? AND login=?", r.PathValue("id"), u.Login).Scan(&id, &status, &ruleID, &eventID, &source, &rev, &created)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	run = map[string]any{"id": id, "status": status, "rule_id": ruleID, "event_id": eventID, "source": source, "rule_revision": rev, "created_at": created}
	rows, _ := s.db.QueryContext(r.Context(), "SELECT id,name,position,type,status,attempt,input_json,output_json,error,started_at,finished_at FROM automation_steps WHERE run_id=? ORDER BY position", id)
	defer rows.Close()
	steps := []map[string]any{}
	for rows.Next() {
		var sid, name, typ, st string
		var pos, attempt int
		var in, out, er, started, finished sql.NullString
		_ = rows.Scan(&sid, &name, &pos, &typ, &st, &attempt, &in, &out, &er, &started, &finished)
		steps = append(steps, map[string]any{"id": sid, "name": name, "position": pos, "type": typ, "status": st, "attempt": attempt, "input_json": in.String, "output_json": out.String, "error": er.String, "started_at": started.String, "finished_at": finished.String})
	}
	jsonResponse(w, 200, map[string]any{"run": run, "steps": steps})
}
func (s *eventService) retryRun(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	res, err := s.db.ExecContext(r.Context(), `UPDATE automation_runs SET status='pending',next_attempt_at=NULL,finished_at=NULL WHERE id=? AND login=? AND status IN ('failed','needs_attention')`, r.PathValue("id"), u.Login)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		http.NotFound(w, r)
		return
	}
	_, _ = s.db.ExecContext(r.Context(), `UPDATE automation_steps SET status='pending',next_attempt_at=NULL,error=NULL WHERE run_id=? AND status IN ('failed','needs_attention')`, r.PathValue("id"))
	w.WriteHeader(204)
}

func (s *eventService) worker() {
	ticker := time.NewTicker(500 * time.Millisecond)
	cleanup := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	defer cleanup.Stop()
	s.cleanup()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.recoverLeases()
			s.runOne()
		case <-cleanup.C:
			s.cleanup()
		}
	}
}
func (s *eventService) recoverLeases() {
	_, _ = s.db.Exec(`UPDATE automation_runs SET status='pending',lease_owner=NULL,lease_expires_at=NULL WHERE status='running' AND lease_expires_at < datetime('now')`)
}
func (s *eventService) cleanup() {
	_, _ = s.db.Exec(`DELETE FROM notifications WHERE created_at < datetime('now','-180 days'); DELETE FROM automation_steps WHERE run_id IN (SELECT id FROM automation_runs WHERE created_at < datetime('now','-180 days')); DELETE FROM automation_runs WHERE created_at < datetime('now','-180 days'); DELETE FROM events WHERE accepted_at < datetime('now','-180 days')`)
	s.disableRetired()
}
func (s *eventService) disableRetired() {
	apps, _, err := manifest.LoadAll(s.root)
	if err != nil {
		return
	}
	active := map[string]bool{}
	for _, a := range apps {
		active[a.Slug] = true
	}
	rows, err := s.db.Query(`SELECT DISTINCT source FROM automation_rules WHERE enabled=1 AND deleted_at IS NULL`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		_ = rows.Scan(&slug)
		if !active[slug] {
			_, _ = s.db.Exec(`UPDATE automation_rules SET enabled=0,enabled_at=NULL,revision=revision+1,updated_at=datetime('now') WHERE source=? AND enabled=1`, slug)
		}
	}
}
func (s *eventService) runOne() {
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	var runID string
	err = tx.QueryRow(`SELECT id FROM automation_runs WHERE (status='pending' AND (next_attempt_at IS NULL OR next_attempt_at<=datetime('now'))) OR (status='running' AND lease_expires_at<datetime('now')) ORDER BY created_at LIMIT 1`).Scan(&runID)
	if err != nil {
		return
	}
	res, err := tx.Exec(`UPDATE automation_runs SET status='running',lease_owner=?,lease_expires_at=datetime('now','+150 seconds'),started_at=COALESCE(started_at,datetime('now')) WHERE id=?`, s.workerID, runID)
	if err != nil {
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return
	}
	if tx.Commit() != nil {
		return
	}
	if err := s.executeRun(runID); err != nil {
		return
	}
}
func (s *eventService) executeRun(runID string) error {
	done := make(chan struct{})
	defer close(done)
	go func() {
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				_, _ = s.db.Exec(`UPDATE automation_runs SET lease_expires_at=datetime('now','+150 seconds') WHERE id=? AND status='running' AND lease_owner=?`, runID, s.workerID)
			}
		}
	}()
	var login, eventID, source string
	err := s.db.QueryRow(`SELECT login,event_id,event_source FROM automation_runs WHERE id=?`, runID).Scan(&login, &eventID, &source)
	if err != nil {
		return err
	}
	e, err := s.loadEvent(context.Background(), login, source, eventID)
	if err != nil {
		return err
	}
	outputs := map[string]any{}
	rows, err := s.db.Query(`SELECT id,name,type,definition_json,status,attempt FROM automation_steps WHERE run_id=? ORDER BY position`, runID)
	if err != nil {
		return err
	}
	type rec struct {
		id, name, typ, def, status string
		attempt                    int
	}
	var steps []rec
	for rows.Next() {
		var x rec
		_ = rows.Scan(&x.id, &x.name, &x.typ, &x.def, &x.status, &x.attempt)
		steps = append(steps, x)
	}
	rows.Close()
	for _, x := range steps {
		if x.status == "succeeded" {
			var raw sql.NullString
			_ = s.db.QueryRow("SELECT output_json FROM automation_steps WHERE id=?", x.id).Scan(&raw)
			if raw.Valid {
				var v any
				_ = json.Unmarshal([]byte(raw.String), &v)
				outputs[x.name] = v
			}
			continue
		}
		_, _ = s.db.Exec(`UPDATE automation_steps SET status='running',attempt=attempt+1,started_at=datetime('now') WHERE id=?`, x.id)
		var st automationStep
		_ = json.Unmarshal([]byte(x.def), &st)
		out, execErr := s.executeStep(context.Background(), login, source, eventID, e, st, x.id, outputs)
		if execErr != nil {
			attempt := x.attempt + 1
			if attempt < 3 {
				delay := "+5 seconds"
				if attempt > 1 {
					delay = "+30 seconds"
				}
				_, _ = s.db.Exec(`UPDATE automation_steps SET status='pending',error=?,next_attempt_at=datetime('now',?),finished_at=datetime('now') WHERE id=?`, clip(execErr.Error(), 4096), delay, x.id)
				_, _ = s.db.Exec(`UPDATE automation_runs SET status='pending',next_attempt_at=datetime('now',?),lease_owner=NULL,lease_expires_at=NULL WHERE id=?`, delay, runID)
			} else {
				_, _ = s.db.Exec(`UPDATE automation_steps SET status='failed',error=?,finished_at=datetime('now') WHERE id=?`, clip(execErr.Error(), 4096), x.id)
				_, _ = s.db.Exec(`UPDATE automation_runs SET status='failed',finished_at=datetime('now'),lease_owner=NULL,lease_expires_at=NULL WHERE id=?`, runID)
			}
			return execErr
		}
		ob, _ := json.Marshal(out)
		_, _ = s.db.Exec(`UPDATE automation_steps SET status='succeeded',output_json=?,error=NULL,finished_at=datetime('now') WHERE id=?`, clip(string(ob), 32<<10), x.id)
		outputs[x.name] = out
	}
	_, _ = s.db.Exec(`UPDATE automation_runs SET status='succeeded',finished_at=datetime('now'),lease_owner=NULL,lease_expires_at=NULL WHERE id=?`, runID)
	return nil
}
func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (s *eventService) executeStep(ctx context.Context, login, source, eventID string, e bespokeevents.Event, st automationStep, actionID string, outputs map[string]any) (any, error) {
	switch st.Type {
	case "notify":
		n := bespokeevents.Notification{Title: render(st.Title, e, outputs), Body: render(st.Body, e, outputs), AppSlug: st.AppSlug, Path: render(st.Path, e, outputs), GroupKey: render(st.GroupKey, e, outputs)}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		created, inserted, err := s.insertNotification(ctx, tx, login, source, eventID, n, actionID)
		if err != nil {
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		if inserted {
			s.emit(login, created)
		}
		return created, nil
	case "ai_json":
		input := resolveAny(st.Input, e, outputs)
		ib, _ := json.Marshal(input)
		if len(ib) > 32<<10 {
			return nil, fmt.Errorf("ai input too large")
		}
		prompt := st.Instruction + "\nInput JSON:\n" + string(ib)
		cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		text, err := llm.New("automations").Complete(cctx, prompt, llm.WithSystem("Respond with one JSON value and no prose or markdown."))
		if err != nil {
			return nil, err
		}
		if len(text) > 32<<10 {
			return nil, fmt.Errorf("ai output too large")
		}
		var out any
		if err = json.Unmarshal([]byte(strings.TrimSpace(text)), &out); err != nil {
			return nil, fmt.Errorf("invalid AI JSON: %w", err)
		}
		b, _ := json.Marshal(st.Schema)
		var sch jsonschema.Schema
		if err = json.Unmarshal(b, &sch); err != nil {
			return nil, err
		}
		resolved, err := sch.Resolve(nil)
		if err != nil {
			return nil, err
		}
		if err = resolved.Validate(out); err != nil {
			return nil, fmt.Errorf("AI schema mismatch: %w", err)
		}
		return out, nil
	case "tool":
		var tool *appTool
		for _, t := range allAppTools(ctx, s.root) {
			if t.Slug == st.ToolApp && t.Name == st.ToolName && t.Automation != "" && t.Automation != "forbidden" {
				tt := t
				tool = &tt
				break
			}
		}
		if tool == nil {
			return nil, fmt.Errorf("tool unavailable or ineligible")
		}
		args := resolveAny(st.Args, e, outputs)
		b, _ := json.Marshal(args)
		if len(b) > 64<<10 {
			return nil, fmt.Errorf("tool arguments too large")
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, tool.URL, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Tailscale-User-Login", login)
		req.Header.Set("Tailscale-User-Name", login)
		req.Header.Set("Bespoke-Causation-ID", eventID)
		if tool.Automation == "idempotent" {
			req.Header.Set("Idempotency-Key", actionID)
		}
		resp, err := toolClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("tool: %s: %s", resp.Status, strings.TrimSpace(string(body)))
		}
		return string(body), nil
	}
	return nil, fmt.Errorf("unknown step")
}
func render(t string, e bespokeevents.Event, outputs map[string]any) string {
	for {
		a := strings.Index(t, "{{")
		if a < 0 {
			return t
		}
		b := strings.Index(t[a+2:], "}}")
		if b < 0 {
			return t
		}
		b += a + 2
		key := strings.TrimSpace(t[a+2 : b])
		v := lookupRef(key, e, outputs)
		t = t[:a] + fmt.Sprint(v) + t[b+2:]
	}
}
func resolveAny(v any, e bespokeevents.Event, outputs map[string]any) any {
	switch x := v.(type) {
	case string:
		if strings.HasPrefix(x, "$") {
			return lookupRef(strings.TrimPrefix(x, "$"), e, outputs)
		}
		return render(x, e, outputs)
	case map[string]any:
		m := map[string]any{}
		for k, v := range x {
			m[k] = resolveAny(v, e, outputs)
		}
		return m
	case []any:
		a := make([]any, len(x))
		for i, v := range x {
			a[i] = resolveAny(v, e, outputs)
		}
		return a
	}
	return v
}
func lookupRef(key string, e bespokeevents.Event, outputs map[string]any) any {
	if key == "event.subject_id" {
		return e.SubjectID
	}
	if key == "event.id" {
		return e.ID
	}
	if strings.HasPrefix(key, "event.data.") {
		v, _ := eventValue(e, strings.TrimPrefix(key, "event."))
		return v
	}
	if strings.HasPrefix(key, "steps.") {
		p := strings.Split(key, ".")
		if len(p) >= 2 {
			v := outputs[p[1]]
			for _, part := range p[2:] {
				m, ok := v.(map[string]any)
				if !ok {
					return ""
				}
				v = m[part]
			}
			return v
		}
	}
	return ""
}
