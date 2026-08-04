package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"syscall"

	"github.com/bketelsen/bespoke/internal/manifest"
)

// cmdDev runs platformd and every manifest app locally with a fake identity.
// The process list comes from the registry, so `bespoke new` apps join
// automatically — nothing to maintain by hand.
func cmdDev(args []string) error {
	devUser := os.Getenv("BESPOKE_DEV_USER")
	if devUser == "" {
		u, _ := user.Current()
		name := "dev"
		if u != nil {
			name = u.Username
		}
		devUser = name + "@local"
	}

	apps, warnings, err := manifest.LoadAll(".")
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}

	type proc struct {
		name, pkg string
		port      int
	}
	procs := []proc{{"platformd", frameworkPackage("./platform"), 4000}}
	for _, a := range apps {
		pkg := "./apps/" + a.Slug
		if a.Package != "" {
			pkg = a.Package
		}
		procs = append(procs, proc{a.Slug, pkg, a.Port})
	}

	// Same isolation deploy uses: an instance's go.sum covers what the
	// instance itself imports, which excludes platformd's dependencies —
	// under the default -mod=readonly `go run ./platform` fails on missing
	// go.sum entries, and `go mod tidy` prunes them straight back out. Build
	// against a throwaway copy so resolution may write without touching the
	// instance's committed go.mod/go.sum.
	modfile, cleanupModfile, err := prepareBuildModfile()
	if err != nil {
		return err
	}
	defer cleanupModfile()

	var cmds []*exec.Cmd
	stopAll := func() {
		for _, c := range cmds {
			if c.Process != nil {
				// go run re-execs the built binary; signal the process group.
				_ = syscall.Kill(-c.Process.Pid, syscall.SIGTERM)
			}
		}
	}
	for _, p := range procs {
		c := exec.Command("go", "run", "-mod=mod", "-modfile="+modfile, p.pkg)
		c.Env = append(os.Environ(), "BESPOKE_DEV_USER="+devUser)
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := c.Start(); err != nil {
			stopAll()
			return fmt.Errorf("start %s: %w", p.name, err)
		}
		cmds = append(cmds, c)
	}

	fmt.Printf("\n  dashboard  http://localhost:4000\n")
	for _, p := range procs[1:] {
		fmt.Printf("  %-10s http://localhost:%d\n", p.name, p.port)
	}
	fmt.Printf("  identity   %s\n\n", devUser)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	stopAll()
	for _, c := range cmds {
		_ = c.Wait()
	}
	return nil
}
