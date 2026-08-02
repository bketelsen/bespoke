package web

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/bketelsen/bespoke/pkg/auth"
	"github.com/starfederation/datastar-go/datastar"
)

// The per-process change hub (ADR-0022): mutations call Changed(login);
// /_live SSE subscribers for that user get a re-rendered fragment patched
// into the page — chat panels and forms survive because only the live
// region morphs.
var liveHub = struct {
	sync.Mutex
	subs map[string]map[chan struct{}]struct{}
}{subs: map[string]map[chan struct{}]struct{}{}}

func liveSubscribe(login string) (chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	liveHub.Lock()
	if liveHub.subs[login] == nil {
		liveHub.subs[login] = map[chan struct{}]struct{}{}
	}
	liveHub.subs[login][ch] = struct{}{}
	liveHub.Unlock()
	return ch, func() {
		liveHub.Lock()
		delete(liveHub.subs[login], ch)
		liveHub.Unlock()
	}
}

var notifyClient = &http.Client{Timeout: 2 * time.Second}

// Notify wakes this process's /_live subscribers for a user without
// touching the plane — platformd's /notify endpoint feeds cross-process
// changes in through here.
func Notify(login string) {
	liveHub.Lock()
	for ch := range liveHub.subs[login] {
		select {
		case ch <- struct{}{}:
		default: // already pending
		}
	}
	liveHub.Unlock()
}

// Changed announces that the user's data in THIS app changed: local /_live
// subscribers re-render, and platformd is nudged (fire-and-forget) so
// dashboard cards refresh too. Call after every mutation — handlers,
// tools, and intents alike.
func Changed(login string) {
	Notify(login)

	go func() {
		plane := cmp.Or(os.Getenv("BESPOKE_LLM_URL"), "http://127.0.0.1:4001")
		req, err := http.NewRequest(http.MethodPost, plane+"/notify",
			bytes.NewReader([]byte(`{"login":`+jsonString(login)+`}`)))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := notifyClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// Live mounts GET /_live: a Datastar SSE stream that re-renders the app's
// live region on every Changed(login) and patches it in place. Pages
// subscribe from a stable wrapper element:
//
//	<div data-init="@get('/_live')">
//	  <div id="<slug>-live">…initial render…</div>
//	</div>
//
// fragment must render the element with that same id.
func Live(mux *http.ServeMux, fragment func(ctx context.Context, user auth.User) (templ.Component, error)) {
	mux.HandleFunc("GET /_live", func(w http.ResponseWriter, r *http.Request) {
		user := auth.FromContext(r.Context())
		ch, cancel := liveSubscribe(user.Login)
		defer cancel()

		sse := datastar.NewSSE(w, r)
		heartbeat := time.NewTicker(45 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-heartbeat.C:
				// Keep intermediaries from timing out the stream.
				if err := sse.PatchSignals([]byte(`{}`)); err != nil {
					return
				}
			case <-ch:
				c, err := fragment(r.Context(), user)
				if err != nil {
					continue // next change will retry; never kill the stream
				}
				if err := sse.PatchElementTempl(c); err != nil {
					return
				}
			}
		}
	})
}
