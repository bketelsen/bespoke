package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The deploy watcher half of the builder plane (ADR-0023,
// docs/design/builder-plane.md): `bespoke deploywatch` runs on the app host
// as the platform user, triggered by bespoke-deploywatch.path whenever a
// request lands in the deploy spool. Nothing the agent produced is trusted:
// the bundle is fast-forwarded into the canonical clone, `just check` must
// pass, and only then is the commit pushed and the narrow deploy run.

func spoolDir() string {
	if s := os.Getenv("BESPOKE_SPOOL"); s != "" {
		return s
	}
	return "/var/lib/bespoke/spool"
}

type deployRequest struct {
	Run  string `json:"run"`
	Slug string `json:"slug"`
}

type deployResult struct {
	Run        string `json:"run"`
	OK         bool   `json:"ok"`
	Detail     string `json:"detail"`
	DeployedAt string `json:"deployed_at,omitempty"`
}

func cmdDeployWatch(args []string) error {
	spool := spoolDir()
	reqs, err := filepath.Glob(filepath.Join(spool, "deploy", "*.request.json"))
	if err != nil {
		return err
	}
	sort.Strings(reqs)
	for _, path := range reqs {
		processDeploy(spool, path)
	}
	return nil
}

func processDeploy(spool, path string) {
	// Archive the request first: a crash mid-deploy must not retrigger the
	// path unit forever. The result file is the durable outcome.
	data, err := os.ReadFile(path)
	archived := filepath.Join(spool, "archive", filepath.Base(path))
	_ = os.MkdirAll(filepath.Dir(archived), 0o770)
	_ = os.Rename(path, archived)
	if err != nil {
		fmt.Fprintln(os.Stderr, "deploywatch: read request:", err)
		return
	}
	var req deployRequest
	if err := json.Unmarshal(data, &req); err != nil || req.Run == "" || req.Slug == "" {
		fmt.Fprintln(os.Stderr, "deploywatch: bad request", path, err)
		return
	}

	runDir := filepath.Join(spool, "runs", req.Run)
	logEvent := func(text string) {
		line, _ := json.Marshal(map[string]string{
			"ts": time.Now().Format(time.RFC3339), "kind": "deploy", "text": text,
		})
		f, err := os.OpenFile(filepath.Join(runDir, "events.jsonl"),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o660)
		if err == nil {
			_, _ = f.Write(append(line, '\n'))
			f.Close()
		}
		fmt.Println("deploywatch:", text)
	}
	fail := func(detail string) {
		logEvent("FAILED: " + detail)
		writeResult(runDir, deployResult{Run: req.Run, OK: false, Detail: detail})
	}

	git := func(args ...string) (string, error) {
		out, err := exec.Command("git", args...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	bundle := filepath.Join(runDir, "repo.bundle")
	if _, err := os.Stat(bundle); err != nil {
		fail("no bundle: " + err.Error())
		return
	}
	prev, err := git("rev-parse", "HEAD")
	if err != nil {
		fail("rev-parse HEAD: " + prev)
		return
	}
	rollback := func() {
		if out, err := git("reset", "--hard", prev); err != nil {
			logEvent("rollback failed: " + out)
		}
	}

	logEvent("verifying bundle for " + req.Slug)
	if out, err := git("bundle", "verify", bundle); err != nil {
		fail("bundle verify: " + out)
		return
	}
	if out, err := git("fetch", bundle, "main"); err != nil {
		fail("bundle fetch: " + out)
		return
	}
	// The agent's history must extend ours — no rewrites, no merges.
	if _, err := git("merge-base", "--is-ancestor", "HEAD", "FETCH_HEAD"); err != nil {
		fail("bundle does not fast-forward from current main")
		return
	}
	if out, err := git("merge", "--ff-only", "FETCH_HEAD"); err != nil {
		fail("fast-forward: " + out)
		return
	}

	logEvent("running just check")
	if out, err := exec.Command("just", "check").CombinedOutput(); err != nil {
		rollback()
		fail("just check failed:\n" + tail(string(out), 4000))
		return
	}

	logEvent("pushing main")
	if out, err := git("push", "origin", "main"); err != nil {
		rollback()
		fail("push: " + out)
		return
	}

	logEvent("deploying " + req.Slug + " (waits for LLM gateway idle)")
	if err := cmdDeploy([]string{req.Slug, "--edge"}); err != nil {
		// Pushed but not deployed: do NOT roll back main; report honestly.
		fail("deploy: " + err.Error() + " (commit is pushed; rerun `bespoke deploy " + req.Slug + " --edge`)")
		return
	}

	logEvent("live")
	writeResult(runDir, deployResult{
		Run: req.Run, OK: true, Detail: "deployed",
		DeployedAt: time.Now().Format(time.RFC3339),
	})
}

func writeResult(runDir string, res deployResult) {
	data, _ := json.Marshal(res)
	tmp := filepath.Join(runDir, "deploy.json.tmp")
	if err := os.WriteFile(tmp, data, 0o660); err != nil {
		fmt.Fprintln(os.Stderr, "deploywatch: write result:", err)
		return
	}
	_ = os.Rename(tmp, filepath.Join(runDir, "deploy.json"))
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
