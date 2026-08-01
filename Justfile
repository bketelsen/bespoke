# Bespoke task runner. `just` lists recipes; `just dev` is the daily driver.
# Recipes wrap the bespoke CLI (docs/specs/bespoke-cli.md).

_default:
    @just --list --unsorted

# Run platformd + every app locally with a fake identity (reads the manifests)
dev:
    go run ./cmd/bespoke dev

# Scaffold a new app and assign its port
new slug:
    go run ./cmd/bespoke new {{ slug }}

# Registry dump (--json for agents)
list *args:
    go run ./cmd/bespoke list {{ args }}

# vet + tests + the CGO-free linux cross-compile (run before calling work done)
check:
    go vet ./...
    go test ./...
    CGO_ENABLED=0 GOOS=linux go build ./...

# Regenerate templ views + recompile the design system CSS (commit the output)
ui:
    scripts/build-ui.sh

# One-time: install templ/templui/tailwind into ./tools
tools:
    scripts/setup-tools.sh

# Build, ship to selfie, restart with rollback (all apps)
deploy *args:
    go run ./cmd/bespoke deploy --all {{ args }}

# Deploy AND push Caddy routes to the edge host (needed when apps change)
deploy-edge:
    go run ./cmd/bespoke deploy --all --edge

# Tail an app's journal on selfie
logs slug *args:
    go run ./cmd/bespoke logs {{ slug }} {{ args }}

# Remove local build output and app data
clean:
    rm -rf dist data
