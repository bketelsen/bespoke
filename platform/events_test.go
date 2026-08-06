package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bespokeevents "github.com/bketelsen/bespoke/pkg/events"
)

func testEventService(t *testing.T) *eventService {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"0001_briefs.sql", "0002_events_automations.sql"} {
		b, err := os.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(b)); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { db.Close() })
	return &eventService{db: db, root: "..", domain: "example.test", dev: true, subs: map[string]map[chan bespokeevents.Notification]struct{}{}, workerID: "test", stop: make(chan struct{})}
}

func publishBody(id, user string, data any) []byte {
	b, _ := json.Marshal(map[string]any{"source": "todo", "user": user, "event": map[string]any{"id": id, "type": "todo.task_created", "occurred_at": time.Now().UTC(), "data": data, "notification": map[string]any{"title": "Created", "app_slug": "todo", "path": "/"}}})
	return b
}

func TestEventPublishIsIdempotentAndDurable(t *testing.T) {
	s := testEventService(t)
	mux := http.NewServeMux()
	s.registerInternal(mux)
	id := "c87fbc45-2478-4434-9e91-6fb6c25f2d70"
	call := func(body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/events/publish", bytes.NewReader(body))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if w := call(publishBody(id, "a@example", map[string]any{"x": 1})); w.Code != 202 {
		t.Fatalf("first=%d %s", w.Code, w.Body.String())
	}
	if w := call(publishBody(id, "a@example", map[string]any{"changed": true})); w.Code != 200 {
		t.Fatalf("retry=%d %s", w.Code, w.Body.String())
	}
	var events, notes int
	_ = s.db.QueryRow("SELECT count(*) FROM events").Scan(&events)
	_ = s.db.QueryRow("SELECT count(*) FROM notifications").Scan(&notes)
	if events != 1 || notes != 1 {
		t.Fatalf("events=%d notifications=%d", events, notes)
	}
}

func TestEventPublishLimitsAndCausation(t *testing.T) {
	s := testEventService(t)
	mux := http.NewServeMux()
	s.registerInternal(mux)
	big := strings.Repeat("x", 65<<10)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/events/publish", bytes.NewReader(publishBody("88c5ad85-73bb-46c3-b9b1-39d421136541", "a", map[string]any{"x": big}))))
	if w.Code != 413 {
		t.Fatalf("large=%d %s", w.Code, w.Body.String())
	}
	_, _ = s.db.Exec(`INSERT INTO events(source,id,login,type,subject_id,occurred_at,data_json,correlation_id,depth) VALUES('todo','11111111-1111-4111-8111-111111111111','a','todo.task_created','',datetime('now'),'{}','11111111-1111-4111-8111-111111111111',8)`)
	b := publishBody("22222222-2222-4222-8222-222222222222", "a", map[string]any{})
	var p map[string]any
	_ = json.Unmarshal(b, &p)
	p["causation_id"] = "11111111-1111-4111-8111-111111111111"
	b, _ = json.Marshal(p)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/events/publish", bytes.NewReader(b)))
	if w.Code != 409 {
		t.Fatalf("depth=%d %s", w.Code, w.Body.String())
	}
}

func TestNotificationPlaneScopesByAssertedUser(t *testing.T) {
	s := testEventService(t)
	mux := http.NewServeMux()
	s.registerInternal(mux)
	id := "33333333-3333-4333-8333-333333333333"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/events/publish", bytes.NewReader(publishBody(id, "alice", map[string]any{}))))
	for user, want := range map[string]int{"alice": 1, "bob": 0} {
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", "/events/notifications?user="+user, nil))
		var p bespokeevents.Page
		_ = json.Unmarshal(w.Body.Bytes(), &p)
		if len(p.Notifications) != want {
			t.Fatalf("%s got %d", user, len(p.Notifications))
		}
	}
}

func TestDeterministicConditionMatching(t *testing.T) {
	e := bespokeevents.Event{SubjectID: "7", Data: map[string]any{"sender": "school", "score": json.Number("4")}}
	if !matches([]condition{{Path: "data.sender", Operator: "equals", Value: "school"}, {Path: "data.score", Operator: "greater_than", Value: json.Number("3")}}, e) {
		t.Fatal("wanted match")
	}
	if matches([]condition{{Path: "data.missing", Operator: "equals", Value: "x"}}, e) {
		t.Fatal("missing path matched")
	}
}

func TestAIJSONRejectsSchemaMismatch(t *testing.T) {
	s := testEventService(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"text": `{"count":"not-a-number"}`})
	}))
	defer srv.Close()
	t.Setenv("BESPOKE_LLM_URL", srv.URL)
	_, err := s.executeStep(t.Context(), "alice", "todo", "event", bespokeevents.Event{ID: "event", Data: map[string]any{}}, automationStep{
		Name: "extract", Type: "ai_json", Instruction: "extract", Input: map[string]any{},
		Schema: map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "properties": map[string]any{"count": map[string]any{"type": "number"}}, "required": []string{"count"}},
	}, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "schema mismatch") {
		t.Fatalf("err=%v", err)
	}
}

func TestExpiredLeaseReturnsToPending(t *testing.T) {
	s := testEventService(t)
	_, err := s.db.Exec(`INSERT INTO automation_runs(id,login,rule_id,event_id,event_source,rule_revision,rule_json,status,lease_expires_at) VALUES('r','a','rule','e','todo',1,'{}','running',datetime('now','-1 second'))`)
	if err != nil {
		t.Fatal(err)
	}
	s.recoverLeases()
	var status string
	_ = s.db.QueryRow("SELECT status FROM automation_runs WHERE id='r'").Scan(&status)
	if status != "pending" {
		t.Fatalf("status=%s", status)
	}
}
