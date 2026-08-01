#!/usr/bin/env bash
# Phase 1 deploy: dev machine → selfie (apps) + edge (Caddy routes).
# Replaced by `bespoke deploy` in Phase 4 (docs/specs/bespoke-cli.md).
#
# Usage:
#   scripts/deploy.sh          # build + deploy apps to selfie
#   scripts/deploy.sh --edge   # also push Caddy routes to the edge host
set -euo pipefail
cd "$(dirname "$0")/.."
source deploy/deploy.env

BINS=(platformd hello)
SRCS=(./platform ./apps/hello)

echo "==> building (linux/${GOARCH})"
mkdir -p dist/bin
for i in "${!BINS[@]}"; do
  CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -o "dist/bin/${BINS[$i]}" "${SRCS[$i]}"
done

echo "==> syncing to ${SELFIE_SSH}:~/bespoke"
ssh "$SELFIE_SSH" 'mkdir -p ~/bespoke/bin ~/bespoke/apps ~/.config/systemd/user'
rsync -az dist/bin/ "$SELFIE_SSH:bespoke/bin/"
rsync -az --include='*/' --include='app.toml' --exclude='*' apps/ "$SELFIE_SSH:bespoke/apps/"
rsync -az deploy/systemd/ "$SELFIE_SSH:.config/systemd/user/"

# One-time env file on selfie; never overwritten here.
ssh "$SELFIE_SSH" "test -f ~/bespoke/env || printf 'BESPOKE_BIND_IP=%s\nBESPOKE_DOMAIN=%s\n' '$SELFIE_TS_IP' '$DOMAIN' > ~/bespoke/env"

echo "==> restarting units"
ssh "$SELFIE_SSH" 'systemctl --user daemon-reload && systemctl --user enable --now bespoke-platformd bespoke-hello && systemctl --user restart bespoke-platformd bespoke-hello'

echo "==> health checks"
for port in 4000 4101; do
  ssh "$SELFIE_SSH" "curl -fsS --max-time 5 http://$SELFIE_TS_IP:$port/healthz >/dev/null" \
    && echo "    :$port ok" || { echo "    :$port FAILED"; exit 1; }
done

if [[ "${1:-}" == "--edge" ]]; then
  echo "==> pushing Caddy routes to ${EDGE_SSH}:${EDGE_CADDY_FILE}"
  # Requires sudo on the edge host; run `ssh -t` manually if it prompts.
  sed -e "s/__DOMAIN__/$DOMAIN/g" -e "s/__SELFIE_TS_IP__/$SELFIE_TS_IP/g" \
    deploy/caddy/bespoke.caddy | ssh "$EDGE_SSH" "sudo tee $EDGE_CADDY_FILE >/dev/null && sudo systemctl reload caddy"
  echo "    done"
fi

echo "==> deployed. Dashboard: https://$DOMAIN"
