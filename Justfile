# Bespoke task runner. `just` lists recipes; `just dev` is the daily driver.

# Fake identity for local runs (no edge proxy locally — see pkg/auth).
dev_user := env_var_or_default("BESPOKE_DEV_USER", `whoami` + "@local")

_default:
    @just --list --unsorted

# Run platformd + all apps locally with a fake identity
dev:
    #!/usr/bin/env bash
    set -euo pipefail
    export BESPOKE_DEV_USER="{{ dev_user }}"
    trap 'kill 0' INT TERM EXIT
    go run ./platform &
    go run ./apps/hello &
    sleep 1
    echo
    echo "  dashboard  http://localhost:4000"
    echo "  hello      http://localhost:4101"
    echo "  identity   $BESPOKE_DEV_USER"
    echo
    wait

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

# Deploy apps to selfie (Phase 1 script; bespoke CLI replaces it in Phase 4)
deploy:
    scripts/deploy.sh

# Deploy apps AND push Caddy routes to the edge host
deploy-edge:
    scripts/deploy.sh --edge

# Remove local build output and app data
clean:
    rm -rf dist data
