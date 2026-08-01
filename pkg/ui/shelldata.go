package ui

import "context"

// AppLink is one entry in the AppShell's app switcher.
type AppLink struct {
	Name string
	Slug string
	Icon string // Lucide name from the manifest
	URL  string // dev-aware: localhost:<port> or https://<slug>.<domain>
}

// ShellData is platform chrome state, stashed in the request context by
// pkg/web's middleware (ADR-0015) so AppShell renders it with zero app code.
type ShellData struct {
	Apps        []AppLink
	Current     string // slug of the running app; "" for platformd
	HomeURL     string
	ChatEnabled bool
}

type shellKey struct{}

// WithShellData attaches platform chrome state to a request context.
// Called by pkg/web; apps never touch it.
func WithShellData(ctx context.Context, d ShellData) context.Context {
	return context.WithValue(ctx, shellKey{}, d)
}

func shellDataFrom(ctx context.Context) ShellData {
	d, _ := ctx.Value(shellKey{}).(ShellData)
	return d
}
