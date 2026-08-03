package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bketelsen/bespoke/internal/manifest"
)

// cmdDeploy builds and ships per docs/specs/bespoke-cli.md: regenerate
// artifacts → cross-compile → rsync → restart unit → healthz with
// rollback → (--edge) push Caddy routes.
func cmdDeploy(args []string) error {
	all := slices.Contains(args, "--all")
	edge := slices.Contains(args, "--edge")
	var slugs []string
	for _, a := range args {
		if !strings.HasPrefix(a, "--") {
			slugs = append(slugs, a)
		}
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	apps, warnings, err := manifest.LoadAll(".")
	if err != nil {
		return err
	}
	if len(warnings) > 0 {
		return fmt.Errorf("fix manifest warnings before deploying: %v", warnings)
	}

	// Which apps to build+restart. platformd always deploys (it's cheap and
	// its view of the registry must match the shipped manifests).
	targets := apps
	if !all && len(slugs) > 0 {
		targets = nil
		for _, s := range slugs {
			i := slices.IndexFunc(apps, func(a manifest.App) bool { return a.Slug == s })
			if i < 0 {
				return fmt.Errorf("unknown app %q", s)
			}
			targets = append(targets, apps[i])
		}
	}

	if err := generate(cfg, apps); err != nil {
		return err
	}
	modfile, cleanupModfile, err := prepareBuildModfile()
	if err != nil {
		return err
	}
	defer cleanupModfile()

	build := func(slug, pkg string) error {
		fmt.Printf("==> build %s (linux/%s)\n", slug, cfg.GoArch)
		cmd := exec.Command("go", "build", "-mod=mod", "-modfile="+modfile, "-o", "dist/bin/"+slug, pkg)
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+cfg.GoArch)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}
	if err := os.MkdirAll("dist/bin", 0o755); err != nil {
		return err
	}
	if err := build("platformd", frameworkPackage("./platform")); err != nil {
		return err
	}
	// The CLI itself and the builder runner ship too (ADR-0023): the deploy
	// watcher and build runner on the app host run these binaries.
	if err := build("bespoke", frameworkPackage("./cmd/bespoke")); err != nil {
		return err
	}
	if err := build("builder-runner", frameworkPackage("./cmd/builder-runner")); err != nil {
		return err
	}
	for _, a := range targets {
		pkg := "./apps/" + a.Slug
		if a.Package != "" {
			pkg = a.Package
		}
		if err := build(a.Slug, pkg); err != nil {
			return err
		}
	}

	fmt.Println("==> sync to", cfg.SelfieSSH)
	if err := run("ssh", cfg.SelfieSSH, "mkdir -p ~/bespoke/bin.new ~/bespoke/apps ~/.config/systemd/user"); err != nil {
		return err
	}
	// Binaries stage into bin.new; the restart step swaps with rollback.
	if err := run("rsync", "-az", "dist/bin/", cfg.SelfieSSH+":bespoke/bin.new/"); err != nil {
		return err
	}
	if err := run("rsync", "-az", "--include=*/", "--include=app.toml", "--exclude=*", "apps/", cfg.SelfieSSH+":bespoke/apps/"); err != nil {
		return err
	}
	if _, err := os.Stat("assets/styles.css"); err == nil {
		if err := run("ssh", cfg.SelfieSSH, "mkdir -p ~/bespoke/assets"); err != nil {
			return err
		}
		if err := run("rsync", "-az", "assets/styles.css", cfg.SelfieSSH+":bespoke/assets/styles.css"); err != nil {
			return err
		}
	}
	if err := run("rsync", "-az", "dist/gen/units/", cfg.SelfieSSH+":.config/systemd/user/"); err != nil {
		return err
	}
	if err := run("rsync", "-az", "dist/gen/litestream.yml", cfg.SelfieSSH+":bespoke/litestream.yml"); err != nil {
		return err
	}
	if err := run("ssh", cfg.SelfieSSH,
		fmt.Sprintf("test -f ~/bespoke/env || printf 'BESPOKE_BIND_IP=%%s\\nBESPOKE_DOMAIN=%%s\\nBESPOKE_LLM_URL=http://%%s:4001\\nBESPOKE_ROOT=%%s/bespoke\\nBESPOKE_LEMONADE_URL=http://127.0.0.1:13305/api/v1\\n' '%s' '%s' '%s' \"$HOME\" > ~/bespoke/env", cfg.SelfieTSIP, cfg.Domain, cfg.SelfieTSIP)); err != nil {
		return err
	}

	// Builder-plane binaries land in the shared bin dir when the host is
	// bootstrapped (deploy/bootstrap-builder.sh); the path unit gets enabled
	// idempotently, and the shared env file learns the spool path so the
	// builder APP writes requests where the runner watches. A host without
	// the bootstrap skips this silently.
	if err := run("ssh", cfg.SelfieSSH, `if [ -d /var/lib/bespoke/bin ]; then
  mv -f ~/bespoke/bin.new/bespoke /var/lib/bespoke/bin/bespoke
  mv -f ~/bespoke/bin.new/builder-runner /var/lib/bespoke/bin/builder-runner
  grep -q '^BESPOKE_SPOOL=' ~/bespoke/env || echo 'BESPOKE_SPOOL=/var/lib/bespoke/spool' >> ~/bespoke/env
  systemctl --user daemon-reload
  systemctl --user enable --now bespoke-deploywatch.path >/dev/null 2>&1 || true
fi`); err != nil {
		return err
	}

	// Quiesce (ADR-0023): never restart units while a completion is in
	// flight — someone may be mid-chat. Polled over ssh because the ACL
	// blocks 4001 from everywhere but the edge host. Unreachable gateway
	// (first deploy, platformd down) proceeds immediately.
	if !slices.Contains(args, "--no-wait") {
		fmt.Println("==> waiting for LLM gateway to go idle")
		if err := run("ssh", cfg.SelfieSSH, fmt.Sprintf(`for i in $(seq 1 90); do
  a=$(curl -fsS --max-time 2 http://%s:4001/llm/activity 2>/dev/null) || exit 0
  case "$a" in *'"inflight":0'*) exit 0 ;; esac
  sleep 2
done
echo "LLM gateway still busy after 180s; deploying anyway" >&2`, cfg.SelfieTSIP)); err != nil {
			return err
		}
	}

	// Reconcile the installed registry after quiescing. Removed apps are
	// stopped and their generated runtime artifacts are deleted, but their
	// databases are deliberately preserved under ~/bespoke/data.
	if err := retireRemovedApps(cfg.SelfieSSH, apps); err != nil {
		return err
	}

	restart := func(slug string, port int) error {
		fmt.Printf("==> restart %s\n", slug)
		// Swap in the new binary, keep the old as .prev, roll back if
		// healthz doesn't come up within 10s.
		script := fmt.Sprintf(`set -e
cd ~/bespoke
[ -f bin/%[1]s ] && cp bin/%[1]s bin/%[1]s.prev || true
mkdir -p bin && mv bin.new/%[1]s bin/%[1]s
systemctl --user daemon-reload
systemctl --user enable --now bespoke-%[1]s >/dev/null 2>&1 || true
systemctl --user restart bespoke-%[1]s
for i in $(seq 1 20); do
  curl -fsS --max-time 1 http://%[2]s:%[3]d/healthz >/dev/null 2>&1 && exit 0
  sleep 0.5
done
echo "healthz failed, rolling back %[1]s" >&2
[ -f bin/%[1]s.prev ] && mv bin/%[1]s.prev bin/%[1]s && systemctl --user restart bespoke-%[1]s
exit 1`, slug, cfg.SelfieTSIP, port)
		return run("ssh", cfg.SelfieSSH, script)
	}
	if err := restart("platformd", 4000); err != nil {
		return err
	}
	for _, a := range targets {
		if err := restart(a.Slug, a.Port); err != nil {
			return err
		}
	}

	if edge {
		fmt.Println("==> push Caddy routes to", cfg.EdgeSSH)
		routes, err := os.ReadFile("dist/gen/bespoke.caddy")
		if err != nil {
			return err
		}
		cmd := exec.Command("ssh", cfg.EdgeSSH,
			fmt.Sprintf("sudo tee %s >/dev/null && sudo systemctl reload caddy", cfg.EdgeCaddyFile))
		cmd.Stdin = strings.NewReader(string(routes))
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	} else {
		fmt.Println("(routes generated; run with --edge after adding/removing apps)")
	}

	fmt.Printf("==> deployed. Dashboard: https://%s\n", cfg.Domain)
	return nil
}

func prepareBuildModfile() (string, func(), error) {
	mod, err := os.ReadFile("go.mod")
	if err != nil {
		return "", nil, fmt.Errorf("read go.mod: %w", err)
	}
	f, err := os.CreateTemp(".", ".bespoke-deploy-*.mod")
	if err != nil {
		return "", nil, fmt.Errorf("create deploy modfile: %w", err)
	}
	modfile, err := filepath.Abs(f.Name())
	if err == nil {
		_, err = f.Write(mod)
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	sumfile := strings.TrimSuffix(modfile, ".mod") + ".sum"
	cleanup := func() {
		_ = os.Remove(modfile)
		_ = os.Remove(sumfile)
	}
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write deploy modfile: %w", err)
	}
	if sum, readErr := os.ReadFile("go.sum"); readErr == nil {
		if err := os.WriteFile(sumfile, sum, 0o600); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("write deploy sumfile: %w", err)
		}
	} else if !os.IsNotExist(readErr) {
		cleanup()
		return "", nil, fmt.Errorf("read go.sum: %w", readErr)
	}
	return modfile, cleanup, nil
}

func retireRemovedApps(host string, apps []manifest.App) error {
	desired := make([]string, 0, len(apps))
	for _, app := range apps {
		desired = append(desired, app.Slug)
	}
	script := fmt.Sprintf(`desired=' %s '
for manifest in "$HOME"/bespoke/apps/*/app.toml; do
  [ -e "$manifest" ] || continue
  slug=$(basename "$(dirname "$manifest")")
  case "$desired" in
    *" $slug "*) continue ;;
  esac
  echo "==> retire $slug (database preserved)"
  systemctl --user disable --now "bespoke-$slug.service" >/dev/null 2>&1 || true
  rm -f "$HOME/.config/systemd/user/bespoke-$slug.service" \
    "$HOME/bespoke/bin/$slug" "$HOME/bespoke/bin/$slug.prev" "$manifest"
  rmdir "$(dirname "$manifest")" 2>/dev/null || true
done
systemctl --user daemon-reload`, strings.Join(desired, " "))
	return run("ssh", host, script)
}

func run(name string, args ...string) error {
	if name == "ssh" {
		args = append([]string{"-o", "ConnectTimeout=10"}, args...)
	}
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
