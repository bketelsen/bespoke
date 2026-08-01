package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// config is deploy/deploy.env — the dev machine's view of the hosts
// (docs/adr/0011-split-host-deployment.md). No secrets live here.
type config struct {
	Domain        string
	SelfieSSH     string
	SelfieTSIP    string
	EdgeSSH       string
	EdgeCaddyFile string
	GoArch        string
}

func loadConfig() (config, error) {
	f, err := os.Open("deploy/deploy.env")
	if err != nil {
		return config{}, fmt.Errorf("deploy/deploy.env: %w (run from the repo root)", err)
	}
	defer f.Close()

	vals := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok {
			vals[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	c := config{
		Domain:        vals["DOMAIN"],
		SelfieSSH:     vals["SELFIE_SSH"],
		SelfieTSIP:    vals["SELFIE_TS_IP"],
		EdgeSSH:       vals["EDGE_SSH"],
		EdgeCaddyFile: vals["EDGE_CADDY_FILE"],
		GoArch:        vals["GOARCH"],
	}
	if c.Domain == "" || c.SelfieSSH == "" || c.SelfieTSIP == "" {
		return c, fmt.Errorf("deploy/deploy.env: DOMAIN, SELFIE_SSH, SELFIE_TS_IP are required")
	}
	if c.GoArch == "" {
		c.GoArch = "amd64"
	}
	return c, nil
}
