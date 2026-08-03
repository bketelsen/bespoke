// bespoke is the platform CLI (docs/specs/bespoke-cli.md): the only supported
// way to create, run, deploy, and operate apps. Generated artifacts (units,
// Caddy routes, Litestream config) all carry a GENERATED header and are never
// hand-edited.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "local"
)

const usage = `bespoke — personal app platform (docs/specs/bespoke-cli.md)

Usage:
  bespoke init <dir>          create a private, version-pinned instance
         --module <path>      module path for the new instance (required)
         [--platform-version] override the Bespoke version for dev builds
         [--with-builder]     include the optional Builder app
  bespoke upgrade <version>  update an instance's pinned Bespoke release
  bespoke ui                 generate templ output and instance CSS
  bespoke new <slug>          scaffold a new app and assign its port
  bespoke dev                 run platformd + every app locally, fake identity
  bespoke deploy [slug|--all] build, ship to the app host, restart, health-check
         [--edge]             also push generated Caddy routes to the edge host
  bespoke list [--json]       registry dump from the manifests
  bespoke logs <slug> [-f]    tail an app's journal on the app host
  bespoke rm <slug> [--force] retire an app (never deletes databases)
  bespoke gen                 regenerate dist/gen artifacts without deploying
  bespoke deploywatch         drain the deploy spool (run by the path unit on
                              the app host — ADR-0023; not for interactive use)
  bespoke version [--json]    print CLI build provenance
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = cmdInit(os.Args[2:])
	case "upgrade":
		err = cmdUpgrade(os.Args[2:])
	case "ui":
		err = cmdUI(os.Args[2:])
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
	case "deploywatch":
		err = cmdDeployWatch(os.Args[2:])
	case "version":
		err = cmdVersion(os.Args[2:])
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

func buildVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func cmdVersion(args []string) error {
	if len(args) > 1 || (len(args) == 1 && args[0] != "--json") {
		return fmt.Errorf("usage: bespoke version [--json]")
	}
	v := struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		Date    string `json:"date"`
		BuiltBy string `json:"built_by"`
	}{buildVersion(), commit, date, builtBy}
	if len(args) == 1 {
		return json.NewEncoder(os.Stdout).Encode(v)
	}
	fmt.Printf("bespoke %s (commit %s, built %s by %s)\n", v.Version, v.Commit, v.Date, v.BuiltBy)
	return nil
}
