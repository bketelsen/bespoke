package web

import (
	"net/http"

	"github.com/starfederation/datastar-go/datastar"
)

// SSE is the Datastar server-sent-events generator apps use for live UI
// (patch elements/signals from handlers). Kept as an alias so apps import
// only pkg/web; the AppShell already loads the matching datastar.js.
type SSE = datastar.ServerSentEventGenerator

// NewSSE starts a Datastar SSE response for r.
func NewSSE(w http.ResponseWriter, r *http.Request) *SSE {
	return datastar.NewSSE(w, r)
}
