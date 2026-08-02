package ui

import "context"

// AppLink is one entry in the AppShell's app switcher.
type AppLink struct {
	Name string
	Slug string
	Icon string // Lucide name from the manifest
	URL  string // dev-aware: localhost:<port> or https://<slug>.<domain>
}

// IntentLink is another app's declared intent, resolved to an invokable URL
// (ADR-0018). The chrome's selection popover and app-level follow-ups
// (ui.IntentsFrom) both consume these.
type IntentLink struct {
	App   string // target app slug
	Name  string // intent name within that app
	Title string // "Create Todo"
	URL   string // absolute confirm-page URL (append ?text=…)
}

// ShellData is platform chrome state, stashed in the request context by
// pkg/web's middleware (ADR-0015) so AppShell renders it with zero app code.
type ShellData struct {
	Apps        []AppLink
	Current     string // slug of the running app; "" for platformd
	HomeURL     string
	ChatEnabled bool
	Intents     []IntentLink // other apps' intents (never the current app's)
}

// IntentsFrom exposes foreign intents to app views for event-driven
// follow-ups (e.g. todo's "Journal it?" banner). Returns nil when none.
func IntentsFrom(ctx context.Context) []IntentLink {
	return shellDataFrom(ctx).Intents
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
