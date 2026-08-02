// builder-runner is the agent half of the builder plane (ADR-0023,
// docs/design/builder-plane.md). It runs on the app host as the unprivileged
// `builder` user, triggered by a path unit when a build request lands in the
// spool. Per request it clones the repo, drives one agentic Copilot session
// in the clone (the repo's AGENTS.md and skills are the agent's instruction
// surface, same as any human-driven agent), verifies the agent committed,
// and hands the commits to the platform side as a git bundle. It holds no
// push credentials and cannot reach production state — that is the point.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

const (
	sessionTimeout = 45 * time.Minute
	testPort       = 42101 // loopback sandbox port; outside manifest + ACL ranges
)

type buildRequest struct {
	Run  string `json:"run"`
	Slug string `json:"slug"`
	Idea string `json:"idea"`
	Spec string `json:"spec_markdown"`
}

type buildStatus struct {
	Run    string `json:"run"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func main() {
	spool := os.Getenv("BESPOKE_SPOOL")
	if spool == "" {
		spool = "/var/lib/bespoke/spool"
	}
	reqs, err := filepath.Glob(filepath.Join(spool, "build", "*.request.json"))
	if err != nil {
		log.Fatal(err)
	}
	sort.Strings(reqs)
	for _, path := range reqs {
		process(spool, path)
	}
}

func process(spool, path string) {
	// Archive first so a crash can't retrigger the path unit forever. The
	// prefix keeps build and deploy archives of the same run apart.
	data, err := os.ReadFile(path)
	archived := filepath.Join(spool, "archive", "build-"+filepath.Base(path))
	_ = os.MkdirAll(filepath.Dir(archived), 0o770)
	_ = os.Rename(path, archived)
	if err != nil {
		log.Println("read request:", err)
		return
	}
	var req buildRequest
	if err := json.Unmarshal(data, &req); err != nil || req.Run == "" || req.Slug == "" {
		log.Println("bad request:", path, err)
		return
	}

	runDir := filepath.Join(spool, "runs", req.Run)
	if err := os.MkdirAll(runDir, 0o770); err != nil {
		log.Println("mkdir run dir:", err)
		return
	}
	events := newEventLog(filepath.Join(runDir, "events.jsonl"))
	defer events.close()

	finish := func(ok bool, detail string) {
		kind := "status"
		if !ok {
			kind = "error"
		}
		events.write(kind, detail)
		data, _ := json.Marshal(buildStatus{Run: req.Run, OK: ok, Detail: detail})
		tmp := filepath.Join(runDir, "status.json.tmp")
		if err := os.WriteFile(tmp, data, 0o660); err == nil {
			_ = os.Rename(tmp, filepath.Join(runDir, "status.json"))
		}
	}

	repoURL := os.Getenv("BESPOKE_REPO_URL")
	if repoURL == "" {
		finish(false, "BESPOKE_REPO_URL is not set in the runner unit")
		return
	}

	home, _ := os.UserHomeDir()
	workDir := filepath.Join(home, "runs", req.Run)
	// A rerun of the same request must not trip over the previous attempt's
	// clone — the workdir is this run's disposable scratch.
	if err := os.RemoveAll(workDir); err != nil {
		finish(false, "clear workdir: "+err.Error())
		return
	}
	repoDir := filepath.Join(workDir, "repo")
	events.write("status", "cloning repository")
	if out, err := exec.Command("git", "clone", repoURL, repoDir).CombinedOutput(); err != nil {
		finish(false, "clone: "+strings.TrimSpace(string(out)))
		return
	}
	git := func(args ...string) (string, error) {
		out, err := exec.Command("git", append([]string{"-C", repoDir}, args...)...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	_, _ = git("config", "user.name", "Bespoke Builder")
	_, _ = git("config", "user.email", "builder@bespoke.local")

	events.write("status", "starting agent session")
	if err := runAgent(repoDir, req, events); err != nil {
		finish(false, "agent session: "+err.Error())
		return
	}

	// Trust nothing yet — just establish the agent produced commits and the
	// app it was asked for. `just check` runs platform-side in the watcher.
	if out, err := git("log", "origin/main..main", "--oneline"); err != nil || out == "" {
		finish(false, "agent made no commits on main")
		return
	}
	if _, err := os.Stat(filepath.Join(repoDir, "apps", req.Slug, "app.toml")); err != nil {
		finish(false, "agent did not create apps/"+req.Slug)
		return
	}

	events.write("status", "bundling commits")
	if out, err := git("bundle", "create", filepath.Join(runDir, "repo.bundle"), "origin/main..main"); err != nil {
		finish(false, "bundle: "+out)
		return
	}
	if err := os.Chmod(filepath.Join(runDir, "repo.bundle"), 0o660); err != nil {
		log.Println("chmod bundle:", err)
	}
	finish(true, "built and bundled; ready to deploy")
}

func runAgent(repoDir string, req buildRequest, events *eventLog) error {
	client := copilot.NewClient(&copilot.ClientOptions{WorkingDirectory: repoDir})
	ctx, cancel := context.WithTimeout(context.Background(), sessionTimeout)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("copilot runtime: %w (is the builder user's copilot CLI authenticated?)", err)
	}
	defer func() { _ = client.Stop() }()

	// The whole session runs as the unprivileged builder user — approving
	// every permission request is the design, not a hole (ADR-0023).
	// ApproveOnce is the valid interactive reply; the bare Approved variant
	// is a result record and the orchestrator rejects it.
	approveAll := func(_ copilot.PermissionRequest, _ copilot.PermissionInvocation) (rpc.PermissionDecision, error) {
		return &rpc.PermissionDecisionApproveOnce{}, nil
	}
	sess, err := client.CreateSession(ctx, &copilot.SessionConfig{
		ClientName:          "bespoke-builder",
		WorkingDirectory:    repoDir,
		OnPermissionRequest: approveAll,
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	defer func() {
		dctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.DeleteSession(dctx, sess.SessionID)
	}()

	unsub := sess.On(func(ev copilot.SessionEvent) {
		switch d := ev.Data.(type) {
		case *copilot.AssistantMessageData:
			events.write("agent", d.Content)
		case *copilot.SessionErrorData:
			events.write("error", d.Message)
		case *copilot.ToolExecutionStartData:
			if args, err := json.Marshal(d.Arguments); err == nil && len(args) > 2 {
				events.write("tool", tail(string(args), 500))
			}
		}
	})
	defer unsub()

	prompt := buildPrompt(req)
	if _, err := sess.SendAndWait(ctx, copilot.MessageOptions{Prompt: prompt}); err != nil {
		return err
	}
	return nil
}

func buildPrompt(req buildRequest) string {
	return fmt.Sprintf(`Build a new Bespoke app with slug %q from the approved spec below.

Follow the repository instructions (AGENTS.md) and the build procedure in
.agents/skills/new-app/SKILL.md exactly. The spec is already approved — do
not redesign it; put it in apps/%s/README.md.

Constraints for this unattended run:
- Scaffold with: go run ./cmd/bespoke new %s
- If UI tooling is missing, run scripts/setup-tools.sh once, and always run
  scripts/build-ui.sh after changing .templ files or the theme, committing
  the generated files.
- Verify the app end to end by running it on the loopback sandbox port:
  BESPOKE_LISTEN=127.0.0.1:%d BESPOKE_DATA=$PWD/../sandbox-data BESPOKE_DEV_USER=%s go run ./apps/%s
  and exercising its routes with curl. NEVER bind any port in 4000-4999 and
  never connect to other ports on this host. Stop the sandbox process when done.
- `+"`just check`"+` must pass before you finish.
- Commit everything to the main branch (you are in a disposable clone; do
  not push — the platform pushes after re-verifying).
- Do not modify any existing app, the platform, or the docs beyond what the
  new-app skill requires.

The user's original idea: %s

Approved spec:

%s`, req.Slug, req.Slug, req.Slug, testPort, "builder@sandbox", req.Slug, req.Idea, req.Spec)
}

type eventLog struct {
	f *os.File
}

func newEventLog(path string) *eventLog {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o660)
	if err != nil {
		log.Println("event log:", err)
		return &eventLog{}
	}
	return &eventLog{f: f}
}

func (l *eventLog) write(kind, text string) {
	if l.f == nil {
		return
	}
	line, _ := json.Marshal(map[string]string{
		"ts": time.Now().Format(time.RFC3339), "kind": kind, "text": text,
	})
	_, _ = l.f.Write(append(line, '\n'))
}

func (l *eventLog) close() {
	if l.f != nil {
		l.f.Close()
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
