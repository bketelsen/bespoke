package main

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/bketelsen/bespoke/internal/manifest"
	"github.com/bketelsen/bespoke/pkg/llm"
)

// The interview is the design-app skill run through the gateway: one
// question per turn, 5–8 questions total, then a spec. The reply protocol
// is mechanical so the app can detect the hand-off: a normal turn is plain
// text; the final turn starts with "SPEC_READY slug=<slug>" followed by the
// finished spec markdown.

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,29}$`)

// existingSlugs reads the deployed manifests (BESPOKE_ROOT on the app host,
// the repo root in dev) so the interview never proposes a taken slug.
func existingSlugs() []string {
	root := cmp.Or(os.Getenv("BESPOKE_ROOT"), ".")
	apps, _, err := manifest.LoadAll(root)
	if err != nil {
		return nil
	}
	slugs := make([]string, 0, len(apps))
	for _, a := range apps {
		slugs = append(slugs, a.Slug)
	}
	return slugs
}

func interviewSystem() string {
	return fmt.Sprintf(`You are the Bespoke platform's app designer. A one-line app idea hides a
dozen decisions; interview the user briefly, then write the spec.

Rules:
- ONE question per reply, and nothing else. Never a questionnaire.
- Offer 2-4 concrete lettered options with a recommendation when possible.
- Budget 5-8 questions total, fewer if the idea already answers them. Cover:
  the moment of use, what one record is, the first screen, lifecycle
  (edit/delete/resurfacing), and 2-3 explicit non-goals.
- If the user says "just build it", stop asking and write the spec.
- After you deliver a spec the user may reply with feedback instead of
  approving. Fold it in and resend the COMPLETE revised spec via the same
  SPEC_READY protocol — never a diff, never a fragment. Keep the slug
  unless the feedback asks for a different one.
- Apps are single-user, mobile-first, one SQLite database, optional LLM
  features (~1.5s/call), optional voice capture. No sharing, no export, no
  auth choices — the platform handles identity.

When you have enough to write every section without guessing, reply with:

SPEC_READY slug=<short-kebab-slug>
# <App Name>

<one sentence: who opens it when, to do what>

## Records
...fields per entity...

## Views
- GET / — <first screen>
...

## Platform surfaces
<dashboard card, chat tools, intents>

## Non-goals
...

## Later
...

The slug must match ^[a-z][a-z0-9-]{1,29}$ and must NOT be one of the
existing apps: %s. Keep the spec under a page. Do not mention the protocol
line to the user.`, strings.Join(existingSlugs(), ", "))
}

// nextTurn sends the transcript to the gateway and returns the assistant's
// reply. Caller stores it; parseSpec decides whether it ended the interview.
func nextTurn(ctx context.Context, ai *llm.Client, db *sql.DB, runID, login string) (string, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT role, body FROM messages WHERE run_id = ? ORDER BY id", runID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var b strings.Builder
	b.WriteString("Interview transcript so far:\n\n")
	for rows.Next() {
		var role, body string
		if err := rows.Scan(&role, &body); err != nil {
			return "", err
		}
		who := "User"
		if role == "assistant" {
			who = "You"
		}
		fmt.Fprintf(&b, "%s: %s\n\n", who, body)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	// The spec itself never enters the messages table (the stored assistant
	// turn is a one-line pointer at the spec card), so on a revision turn the
	// designer needs it restated here.
	var slug, spec string
	_ = db.QueryRowContext(ctx, "SELECT slug, spec FROM runs WHERE id = ?", runID).Scan(&slug, &spec)
	if spec != "" {
		fmt.Fprintf(&b, "The draft spec you already delivered (slug=%s):\n\n%s\n\n", slug, spec)
	}
	b.WriteString("Reply with your next single question, or the finished spec per the protocol.")
	return ai.Complete(ctx, b.String(), llm.WithSystem(interviewSystem()), llm.WithUser(login))
}

// parseSpec returns (slug, spec, true) when the reply ends the interview.
func parseSpec(reply string) (string, string, bool) {
	first, rest, _ := strings.Cut(strings.TrimSpace(reply), "\n")
	if !strings.HasPrefix(first, "SPEC_READY") {
		return "", "", false
	}
	slug := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(first, "SPEC_READY")), "slug="))
	return slug, strings.TrimSpace(rest), true
}

// validSlug rejects malformed and already-taken slugs.
func validSlug(slug string) error {
	if !slugRe.MatchString(slug) {
		return fmt.Errorf("slug %q is malformed", slug)
	}
	for _, s := range existingSlugs() {
		if s == slug {
			return fmt.Errorf("slug %q is already an app", slug)
		}
	}
	return nil
}
