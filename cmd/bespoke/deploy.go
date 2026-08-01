package main

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/bketelsen/bespoke/internal/manifest"
)

// cmdDeploy builds and ships per docs/specs/bespoke-cli.md: cross-compile →
// rsync → restart unit → healthz with rollback → regenerate artifacts →
// (--edge) push Caddy routes. ⚠ Remote paths are UNVALIDATED until the
// Phase 1 runbook has been executed (docs/plans/roadmap.md).
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

	build := func(slug, pkg string) error {
		fmt.Printf("==> build %s (linux/%s)\n", slug, cfg.GoArch)
		cmd := exec.Command("go", "build", "-o", "dist/bin/"+slug, pkg)
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+cfg.GoArch)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}
	if err := os.MkdirAll("dist/bin", 0o755); err != nil {
		return err
	}
	if err := build("platformd", "./platform"); err != nil {
		return err
	}
	for _, a := range targets {
		if err := build(a.Slug, "./apps/"+a.Slug); err != nil {
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
	if err := run("rsync", "-az", "dist/gen/units/", cfg.SelfieSSH+":.config/systemd/user/"); err != nil {
		return err
	}
	if err := run("rsync", "-az", "dist/gen/litestream.yml", cfg.SelfieSSH+":bespoke/litestream.yml"); err != nil {
		return err
	}
	if err := run("ssh", cfg.SelfieSSH,
		fmt.Sprintf("test -f ~/bespoke/env || printf 'BESPOKE_BIND_IP=%%s\\nBESPOKE_DOMAIN=%%s\\n' '%s' '%s' > ~/bespoke/env", cfg.SelfieTSIP, cfg.Domain)); err != nil {
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

func run(name string, args ...string) error {
	if name == "ssh" {
		args = append([]string{"-o", "ConnectTimeout=10"}, args...)
	}
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
