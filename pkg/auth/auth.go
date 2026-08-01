// Package auth is the only way apps learn who is calling
// (docs/adr/0004-tailscale-identity-via-caddy.md). Identity arrives as
// Tailscale-User-* headers set by the edge proxy; the Tailscale ACL
// guarantees nothing else can reach an app port (ADR-0011).
package auth

import (
	"cmp"
	"context"
	"net/http"
	"os"
)

type User struct {
	Login string // e.g. "bketelsen@github" — stable identifier
	Name  string // display name; falls back to Login
}

type ctxKey struct{}

// FromContext returns the authenticated user. It panics if the handler was
// not wrapped by Middleware — that is a wiring bug, not a runtime condition.
func FromContext(ctx context.Context) User {
	u, ok := ctx.Value(ctxKey{}).(User)
	if !ok {
		panic("auth.FromContext: handler not wrapped by auth.Middleware")
	}
	return u
}

// Middleware rejects requests without an identity header and stores the User
// in the request context. For local development, set BESPOKE_DEV_USER to
// bypass the edge proxy (e.g. BESPOKE_DEV_USER=me@github).
func Middleware(next http.Handler) http.Handler {
	dev := os.Getenv("BESPOKE_DEV_USER")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		login := r.Header.Get("Tailscale-User-Login")
		name := r.Header.Get("Tailscale-User-Name")
		if login == "" && dev != "" {
			login, name = dev, dev
		}
		if login == "" {
			http.Error(w, "no identity header; expected to be reached via the edge proxy", http.StatusUnauthorized)
			return
		}
		u := User{Login: login, Name: cmp.Or(name, login)}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}
