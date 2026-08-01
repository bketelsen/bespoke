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
	procs := []proc{{"platformd", "./platform", 4000}}
	for _, a := range apps {
		procs = append(procs, proc{a.Slug, "./apps/" + a.Slug, a.Port})
	}

	var cmds []*exec.Cmd
	stopAll := func() {
		for _, c := range cmds {
			if c.Process != nil {
				// go run re-execs the built binary; signal the process group.
				syscall.Kill(-c.Process.Pid, syscall.SIGTERM)
			}
		}
	}
	for _, p := range procs {
		c := exec.Command("go", "run", p.pkg)
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
		c.Wait()
	}
	return nil
}
