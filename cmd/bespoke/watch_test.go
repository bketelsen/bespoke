package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitIn returns a git runner bound to dir with a fixed identity, the same
// shape applyBundle takes.
func gitIn(t *testing.T, dir string) func(...string) (string, error) {
	t.Helper()
	return func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
}

func mustGit(t *testing.T, git func(...string) (string, error), args ...string) string {
	t.Helper()
	out, err := git(args...)
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return out
}

func commitFile(t *testing.T, dir string, git func(...string) (string, error), name, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, git, "add", name)
	mustGit(t, git, "commit", "-m", msg)
}

// bundleWorld builds the builder-plane git topology: an upstream, the
// canonical clone (where applyBundle runs), and a runner clone that commits
// an app and bundles it. Returns the canonical dir+git and the bundle path.
func bundleWorld(t *testing.T) (canonical string, git func(...string) (string, error), runnerDir string, runnerGit func(...string) (string, error), bundle string) {
	t.Helper()
	root := t.TempDir()

	upstream := filepath.Join(root, "upstream")
	upGit := gitIn(t, upstream)
	if err := os.MkdirAll(upstream, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, upGit, "init", "-q", "-b", "main")
	commitFile(t, upstream, upGit, "base.txt", "base\n", "base")

	canonical = filepath.Join(root, "canonical")
	git = gitIn(t, root)
	mustGit(t, git, "clone", "-q", upstream, canonical)
	git = gitIn(t, canonical)

	runnerDir = filepath.Join(root, "runner")
	runnerGit = gitIn(t, root)
	mustGit(t, runnerGit, "clone", "-q", upstream, runnerDir)
	runnerGit = gitIn(t, runnerDir)
	commitFile(t, runnerDir, runnerGit, "app.txt", "the app\n", "runner: build app")

	bundle = filepath.Join(root, "repo.bundle")
	mustGit(t, runnerGit, "bundle", "create", bundle, "origin/main..main")
	return
}

func noopLog(string) {}

func TestApplyBundleFastForward(t *testing.T) {
	canonical, git, _, _, bundle := bundleWorld(t)
	if err := applyBundle(git, bundle, noopLog); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(canonical, "app.txt")); err != nil {
		t.Error("app.txt missing after fast-forward apply")
	}
}

func TestApplyBundleRebasesStaleBundle(t *testing.T) {
	canonical, git, _, _, bundle := bundleWorld(t)
	// Something lands on main while the run was building.
	commitFile(t, canonical, git, "other.txt", "other work\n", "other: landed mid-run")

	var events []string
	logEvent := func(s string) { events = append(events, s) }
	if err := applyBundle(git, bundle, logEvent); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"app.txt", "other.txt", "base.txt"} {
		if _, err := os.Stat(filepath.Join(canonical, f)); err != nil {
			t.Errorf("%s missing after rebased apply", f)
		}
	}
	if n := mustGit(t, git, "rev-list", "--count", "--merges", "HEAD"); n != "0" {
		t.Errorf("history has %s merge commits, want linear", n)
	}
	if len(events) == 0 || !strings.Contains(events[0], "rebasing 1 commit") {
		t.Errorf("expected a rebasing event, got %v", events)
	}
}

func TestApplyBundleConflictFailsClean(t *testing.T) {
	canonical, git, _, _, bundle := bundleWorld(t)
	// Mid-run work that collides with the runner's app.txt.
	commitFile(t, canonical, git, "app.txt", "conflicting content\n", "other: conflicting")
	head := mustGit(t, git, "rev-parse", "HEAD")

	err := applyBundle(git, bundle, noopLog)
	if err == nil || !strings.Contains(err.Error(), "conflicts with current main") {
		t.Fatalf("want conflict error, got %v", err)
	}
	// The sequence was aborted: no cherry-pick in progress, HEAD unmoved.
	if _, statErr := os.Stat(filepath.Join(canonical, ".git", "CHERRY_PICK_HEAD")); statErr == nil {
		t.Error("cherry-pick left in progress")
	}
	if now := mustGit(t, git, "rev-parse", "HEAD"); now != head {
		t.Errorf("HEAD moved across failed apply: %s -> %s", head, now)
	}
}
