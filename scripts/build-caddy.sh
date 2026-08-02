#!/usr/bin/env bash
# Build the edge host's Caddy (caddy-tailscale + caddy-dns/cloudflare) on
# the DEV machine — the edge host has no toolchain, same rule as selfie
# (ADR-0011: hosts receive binaries). Runbook: deploy/README.md.
#
# Usage:
#   scripts/build-caddy.sh          # build dist/caddy
#   scripts/build-caddy.sh --push   # build, then install on the edge host
set -euo pipefail
cd "$(dirname "$0")/.."
source deploy/deploy.env

mkdir -p dist tools
[ -x tools/xcaddy ] || GOBIN="$PWD/tools" go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest

echo "==> building caddy (linux/${EDGE_GOARCH:-amd64}) with tailscale + cloudflare-dns"
GOOS=linux GOARCH="${EDGE_GOARCH:-amd64}" tools/xcaddy build \
  --with github.com/tailscale/caddy-tailscale \
  --with github.com/caddy-dns/cloudflare \
  --output dist/caddy
ls -lh dist/caddy

if [[ "${1:-}" == "--push" ]]; then
  echo "==> installing on $EDGE_SSH (previous binary kept as /usr/bin/caddy.bak)"
  scp dist/caddy "$EDGE_SSH:/tmp/caddy.new"
  # Individual sudo commands, each matching an entry in
  # deploy/sudoers/bespoke-edge exactly. The stock caddy systemd unit
  # grants CAP_NET_BIND_SERVICE, so a plain binary swap is all that's needed.
  ssh -o ConnectTimeout=10 "$EDGE_SSH" '
    sudo cp /usr/bin/caddy /usr/bin/caddy.bak 2>/dev/null || true
    sudo systemctl stop caddy
    sudo install -m 755 /tmp/caddy.new /usr/bin/caddy
    rm -f /tmp/caddy.new
    sudo systemctl start caddy
    systemctl is-active caddy
  '
  echo "==> rollback: ssh $EDGE_SSH 'sudo install -m 755 /usr/bin/caddy.bak /usr/bin/caddy && sudo systemctl restart caddy'"
fi
