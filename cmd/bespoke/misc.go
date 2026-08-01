package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"

	"github.com/bketelsen/bespoke/internal/manifest"
)

// cmdList dumps the registry. --json is the stable agent-facing form
// (docs/specs/bespoke-cli.md).
func cmdList(args []string) error {
	apps, warnings, err := manifest.LoadAll(".")
	if err != nil {
		return err
	}
	if slices.Contains(args, "--json") {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"apps": apps, "warnings": warnings,
		})
	}
	fmt.Printf("%-16s %-6s %s\n", "SLUG", "PORT", "NAME")
	for _, a := range apps {
		fmt.Printf("%-16s %-6d %s\n", a.Slug, a.Port, a.Name)
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	return nil
}

func cmdLogs(args []string) error {
	follow := slices.Contains(args, "-f")
	args = slices.DeleteFunc(args, func(s string) bool { return s == "-f" })
	if len(args) != 1 {
		return fmt.Errorf("usage: bespoke logs <slug> [-f]")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	remote := "journalctl --user -u bespoke-" + args[0] + " -n 100"
	if follow {
		remote += " -f"
	}
	cmd := exec.Command("ssh", "-t", cfg.SelfieSSH, remote)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// cmdRm retires an app: stops the unit and archives remote data, then (with
// --force) deletes the app directory. Never deletes databases
// (docs/specs/bespoke-cli.md).
func cmdRm(args []string) error {
	force := slices.Contains(args, "--force")
	args = slices.DeleteFunc(args, func(s string) bool { return s == "--force" })
	if len(args) != 1 {
		return fmt.Errorf("usage: bespoke rm <slug> [--force]")
	}
	slug := args[0]
	if _, err := manifest.Load(".", slug); err != nil {
		return err
	}

	if cfg, err := loadConfig(); err == nil {
		fmt.Println("==> stopping remote unit and archiving data")
		script := fmt.Sprintf(`systemctl --user disable --now bespoke-%[1]s 2>/dev/null || true
rm -f ~/.config/systemd/user/bespoke-%[1]s.service ~/bespoke/bin/%[1]s ~/bespoke/bin/%[1]s.prev
rm -rf ~/bespoke/apps/%[1]s
mkdir -p ~/bespoke/data/archive
[ -f ~/bespoke/data/%[1]s.db ] && mv ~/bespoke/data/%[1]s.db ~/bespoke/data/archive/ || true
systemctl --user daemon-reload`, slug)
		if err := run("ssh", cfg.SelfieSSH, script); err != nil {
			fmt.Fprintln(os.Stderr, "remote cleanup failed (continuing):", err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "no deploy config; skipping remote cleanup:", err)
	}

	if !force {
		fmt.Printf("apps/%s kept (local data untouched). Re-run with --force to delete the directory,\nthen run `bespoke deploy --all --edge` to drop its route.\n", slug)
		return nil
	}
	if err := os.RemoveAll("apps/" + slug); err != nil {
		return err
	}
	fmt.Printf("removed apps/%s — run `bespoke deploy --all --edge` to drop its route. Local data/%s.db untouched.\n", slug, slug)
	return nil
}
