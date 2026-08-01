// bespoke is the platform CLI (docs/specs/bespoke-cli.md): the only supported
// way to create, run, deploy, and operate apps. Generated artifacts (units,
// Caddy routes, Litestream config) all carry a GENERATED header and are never
// hand-edited.
package main

import (
	"fmt"
	"os"
)

const usage = `bespoke — personal app platform (docs/specs/bespoke-cli.md)

Usage:
  bespoke new <slug>          scaffold a new app and assign its port
  bespoke dev                 run platformd + every app locally, fake identity
  bespoke deploy [slug|--all] build, ship to the app host, restart, health-check
         [--edge]             also push generated Caddy routes to the edge host
  bespoke list [--json]       registry dump from the manifests
  bespoke logs <slug> [-f]    tail an app's journal on the app host
  bespoke rm <slug> [--force] retire an app (never deletes databases)
  bespoke gen                 regenerate dist/gen artifacts without deploying
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "new":
		err = cmdNew(os.Args[2:])
	case "dev":
		err = cmdDev(os.Args[2:])
	case "deploy":
		err = cmdDeploy(os.Args[2:])
	case "list":
		err = cmdList(os.Args[2:])
	case "logs":
		err = cmdLogs(os.Args[2:])
	case "rm":
		err = cmdRm(os.Args[2:])
	case "gen":
		err = cmdGen(os.Args[2:])
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "bespoke:", err)
		os.Exit(1)
	}
}
