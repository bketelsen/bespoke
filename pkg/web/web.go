// Package web is the server scaffold every app runs on
// (docs/design/architecture.md). It owns listening, identity enforcement,
// /healthz, request logging, and graceful shutdown, so apps contain zero
// infrastructure code — just routes.
package web

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/bketelsen/bespoke/internal/manifest"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/bketelsen/bespoke/pkg/ui"
)

// selfBase is the app's own HTTP base (set once listening), used to hand
// the gateway callable tool URLs.
var selfBase atomic.Pointer[string]

// Run serves the app named slug on the port from its manifest — the normal
// entrypoint for apps. Production units override the address with
// -listen <tailscale-ip>:<port> (docs/adr/0011-split-host-deployment.md).
func Run(slug string, register func(mux *http.ServeMux)) {
	root := cmp.Or(os.Getenv("BESPOKE_ROOT"), ".")
	app, err := manifest.Load(root, slug)
	if err != nil {
		log.Fatalf("%s: cannot determine port: %v", slug, err)
	}
	Serve(slug, app.Port, register)
}

// Serve is Run with an explicit default port, for platform processes that
// have no app manifest (platformd). Every route registered by register is
// behind auth.Middleware; /healthz is not. Blocks until SIGINT/SIGTERM,
// then drains for up to 5s.
func Serve(slug string, defaultPort int, register func(mux *http.ServeMux)) {
	def := cmp.Or(os.Getenv("BESPOKE_LISTEN"), fmt.Sprintf("127.0.0.1:%d", defaultPort))
	listen := flag.String("listen", def, "listen address")
	flag.Parse()

	// The app's own callable base — the gateway invokes tools back over
	// HTTP at this address (ADR-0021).
	base := "http://" + *listen
	selfBase.Store(&base)

	mux := http.NewServeMux()
	register(mux)

	outer := http.NewServeMux()
	outer.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	// Design system assets (compiled CSS + component/datastar JS), embedded
	// in every binary; static, so outside auth like /healthz.
	outer.Handle("GET /_bespoke/", ui.Handler())
	outer.Handle("/", auth.Middleware(logRequests(slug, withShellData(slug, mux))))

	srv := &http.Server{Addr: *listen, Handler: outer}
	go func() {
		log.Printf("%s listening on %s", slug, *listen)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func logRequests(slug string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		log.Printf("%s %d %s %s %s", slug, sw.status, r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Flush and Unwrap keep SSE working through the logging wrapper (/_live,
// Datastar): without Flusher passthrough, events buffer forever.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
