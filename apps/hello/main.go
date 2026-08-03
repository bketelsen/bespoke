// hello: the canonical app shape — manifest, main.go, views, migrations,
// zero infrastructure code (CLAUDE.md).
package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/a-h/templ"
	"github.com/bketelsen/bespoke/apps/hello/views"
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
		// Live visit count (ADR-0022): another tab or device visiting bumps
		// the number here without a reload.
		web.Live(mux, func(ctx context.Context, user auth.User) (templ.Component, error) {
			var visits int
			if err := sqldb.QueryRowContext(ctx,
				"SELECT count(*) FROM visits WHERE login = ?", user.Login).Scan(&visits); err != nil {
				return nil, err
			}
			return views.CountLive(visits), nil
		})

		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			user := auth.FromContext(r.Context())
			if _, err := sqldb.Exec("INSERT INTO visits (login) VALUES (?)", user.Login); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			web.Changed(user.Login) // every mutation notifies (ADR-0022)
			var visits int
			if err := sqldb.QueryRow("SELECT count(*) FROM visits WHERE login = ?", user.Login).Scan(&visits); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			views.Home(user, visits).Render(r.Context(), w)
		})
	})
}
