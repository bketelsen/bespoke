# Deploy Runbook — Phase 1

> ⚠ **UNVALIDATED** (2026-08-01): this runbook has not been executed
> end-to-end yet. Expect rough edges on first run — in particular, verify the
> caddy-tailscale placeholder names and the `map {labels.N}` index against
> your domain and plugin version.

Topology per [ADR-0011](../docs/adr/0011-split-host-deployment.md): **edge**
(existing Caddy server) → **selfie** (app host) → built from the **dev
machine**. Everything talks over the tailnet.

## One-time setup

### 1. Edge host — custom Caddy, built on the DEV machine

The edge host has no toolchain (same rule as selfie: hosts receive
binaries). From the dev machine:

```sh
just caddy        # cross-compile dist/caddy with tailscale + cloudflare-dns
just caddy-push   # …and install it on the edge host (old binary → caddy.bak)
```

Set `EDGE_GOARCH` in [deploy.env](deploy.env) if the edge isn't amd64. The
push swaps `/usr/bin/caddy` under systemd (the stock unit already grants
`CAP_NET_BIND_SERVICE`, so a plain binary works) and prints the rollback
one-liner.

On the edge host itself, add to Caddy's environment (e.g. a systemd
drop-in):

```text
CLOUDFLARE_API_TOKEN=<token with Zone:DNS:Edit for the domain>
```

The edge host must be on the tailnet with tailscaled running (the
`tailscale_auth` directive asks it who each connection is).

### 2. Cloudflare DNS

Two records, both pointing at the **edge host's tailscale IPv4** (`tailscale
ip -4` on the edge host). A public record carrying a 100.64/10 address is
fine — it only resolves usefully for tailnet members:

```
A  bespoke.example.com    100.<edge-ip>   (DNS only — grey cloud, NOT proxied)
A  *.bespoke.example.com  100.<edge-ip>   (DNS only — grey cloud, NOT proxied)
```

The wildcard **cert** comes from the ACME DNS challenge via the API token; no
inbound exposure is created.

### 3. Tailscale ACL — the security invariant

Only the edge host may reach app ports on selfie
([ADR-0011](../docs/adr/0011-split-host-deployment.md)). In the tailnet
policy file, ensure no broad rule grants other devices access to
`selfie:4000-4999`, and add:

```jsonc
{
  "action": "accept",
  "src":    ["<edge-hostname-or-tag>"],
  "dst":    ["selfie:4000-4999"]
}
```

Tailscale ACLs are allowlists — audit existing `accept` rules (especially any
`"dst": ["*:*"]`) to confirm they don't already open these ports to everything.

### 4. selfie — app host

```sh
loginctl enable-linger $USER     # user units run without a login session
mkdir -p ~/bespoke
```

`~/bespoke/env` is created by the first deploy (bind IP, domain, LLM plane
URL, and `BESPOKE_LEMONADE_URL=http://127.0.0.1:13305/api/v1` for the audio
backend — adjust if Lemonade's port differs); units, binaries, manifests,
and litestream config are synced by `bespoke deploy`.

For backups (ADR-0007), install [Litestream](https://litestream.io) to
`/usr/local/bin/litestream`, then append to `~/bespoke/env`:

```sh
BESPOKE_DATA_DIR=/home/<user>/bespoke/data
BESPOKE_REPLICA_URL=s3://<bucket>/bespoke        # R2/B2/S3 endpoint
# plus the store's credentials (e.g. LITESTREAM_ACCESS_KEY_ID/SECRET)
```

and `systemctl --user enable --now bespoke-litestream` after the first deploy.

### 5. Passwordless sudo (both hosts)

The tooling escalates only for a fixed set of commands; scoped sudoers
files live in [sudoers/](sudoers/). On each host:

```sh
visudo -cf bespoke-edge   && sudo install -m 0440 bespoke-edge   /etc/sudoers.d/bespoke   # edge
visudo -cf bespoke-selfie && sudo install -m 0440 bespoke-selfie /etc/sudoers.d/bespoke   # selfie
```

Edge covers route pushes + caddy binary swaps; selfie needs no sudo for
routine deploys (user units) — its file only covers linger and litestream
installs. Adjust the username inside if it isn't `bjk`.

### 6. Dev machine

Copy [deploy.env.example](deploy.env.example) to `deploy/deploy.env`
(gitignored — your hosts stay out of the repo) and fill in: domain, ssh
destinations, tailscale IPs, arch (`GOARCH`, `EDGE_GOARCH`). Requires
`go`, `rsync`, `ssh`, `just`.

## Deploying

```sh
just deploy        # bespoke deploy --all: build, ship, restart w/ rollback
just deploy-edge   # also push generated Caddy routes + reload caddy
```

`--edge` is needed on the first deploy and whenever an app is added/removed
(the generated route map in `dist/gen/bespoke.caddy` changes).

## Restore drill (do this once before trusting backups)

On selfie, restore one database to a scratch path and compare:

```sh
litestream restore -config ~/bespoke/litestream.yml -o /tmp/hello-restored.db \
  "$BESPOKE_DATA_DIR/hello.db"
sqlite3 /tmp/hello-restored.db 'SELECT count(*) FROM visits;'   # matches live?
```

## Verify (Phase 1 "done when")

From any tailnet device: `https://hello.<domain>` renders your Tailscale
login name, and `https://<domain>` shows the dashboard with the Hello app.

## Troubleshooting

- **502 from Caddy** — unit down? `ssh selfie systemctl --user status
  bespoke-hello`. Or the ACL blocks edge→selfie: test from the edge host with
  `curl http://<selfie-ts-ip>:4101/healthz`.
- **401 "no identity header"** — request reached the app without passing
  `tailscale_auth`; check the caddy-tailscale placeholder names against the
  plugin version (`{http.auth.user.tailscale_login}`).
- **Cert not issued** — check the Cloudflare token scope and `journalctl -u
  caddy` on the edge host for the ACME DNS challenge result.
