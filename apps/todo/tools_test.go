package main

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/db"
)

func TestCreateToolIdempotency(t *testing.T) {
	t.Setenv("BESPOKE_DATA", t.TempDir())
	t.Setenv("BESPOKE_INTERNAL_URL", "http://127.0.0.1:1")
	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("todo", migrations)
	if err != nil {
		t.Fatal(err)
	}
	defer sqldb.Close()
	mux := http.NewServeMux()
	registerTools(mux, sqldb)
	handler := auth.Middleware(mux)
	call := func() (int, string) {
		r := httptest.NewRequest("POST", "/_tools/create_task", bytes.NewBufferString(`{"description":"one task"}`))
		r.Header.Set("Tailscale-User-Login", "a@example")
		r.Header.Set("Idempotency-Key", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	c1, b1 := call()
	c2, b2 := call()
	if c1 != 200 || c2 != 200 || b1 != b2 {
		t.Fatalf("first=%d %q second=%d %q", c1, b1, c2, b2)
	}
	var n int
	if err = sqldb.QueryRow("SELECT count(*) FROM tasks WHERE login='a@example'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}
}
