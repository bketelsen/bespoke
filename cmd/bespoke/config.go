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
	// ReplicaKeyPath is a path ON THE APP HOST to an SSH private key, for
	// Litestream replicas that authenticate with one (sftp://). A path is not
	// a secret, so it belongs here; the key never leaves the app host. Empty
	// for backends whose credentials ride in the URL or LITESTREAM_* vars.
	ReplicaKeyPath string
	// ReplicaHostKey pins the SFTP server's public host key. Without it
	// Litestream logs "host key not verified" and connects anyway, so anything
	// on the network can impersonate the backup target and collect every
	// database. A public key, not a secret.
	//
	// It must be the host's ECDSA key: Litestream's Go SSH client negotiates
	// ecdsa-sha2-nistp256, so an ed25519 or RSA key here fails as a mismatch.
	//     ssh-keyscan -t ecdsa <host>
	ReplicaHostKey string
}

func loadConfig() (config, error) {
	f, err := os.Open("deploy/deploy.env")
	if err != nil {
		return config{}, fmt.Errorf("deploy/deploy.env: %w (run from the repo root; copy deploy/deploy.env.example and fill in your hosts)", err)
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
		Domain:         vals["DOMAIN"],
		SelfieSSH:      vals["SELFIE_SSH"],
		SelfieTSIP:     vals["SELFIE_TS_IP"],
		EdgeSSH:        vals["EDGE_SSH"],
		EdgeCaddyFile:  vals["EDGE_CADDY_FILE"],
		GoArch:         vals["GOARCH"],
		ReplicaKeyPath: vals["REPLICA_KEY_PATH"],
		ReplicaHostKey: vals["REPLICA_HOST_KEY"],
	}
	if c.Domain == "" || c.SelfieSSH == "" || c.SelfieTSIP == "" {
		return c, fmt.Errorf("deploy/deploy.env: DOMAIN, SELFIE_SSH, SELFIE_TS_IP are required")
	}
	if c.GoArch == "" {
		c.GoArch = "amd64"
	}
	return c, nil
}
