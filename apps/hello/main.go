// hello: the Phase 1 proof app, now on pkg/* (Phase 2: an app is only its
// routes, views, and migrations — zero infrastructure code).
package main

import (
	"embed"
	"fmt"
	"html"
	"io/fs"
	"log"
	"net/http"

	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/db"
	"github.com/bketelsen/bespoke/pkg/web"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func main() {
	migrations, _ := fs.Sub(migrationFS, "migrations")
	sqldb, err := db.Open("hello", migrations)
	if err != nil {
		log.Fatal(err)
	}

	web.Run("hello", func(mux *http.ServeMux) {
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if _, err := sqldb.Exec("INSERT INTO visits (login) VALUES (?)", user.Login); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			var visits int
			if err := sqldb.QueryRow("SELECT count(*) FROM visits WHERE login = ?", user.Login).Scan(&visits); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>Hello</title>
<h1>Hello, %s 👋</h1><p>Authenticated as <code>%s</code> — visit #%d through the whole stack.</p>`,
				html.EscapeString(user.Name), html.EscapeString(user.Login), visits)
		})
	})
}
