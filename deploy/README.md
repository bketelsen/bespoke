# Deploy Runbook — Phase 1

> ✅ **VALIDATED** (2026-08-01): executed end-to-end for the reference
> deployment (`bespoke.ketelsen.cloud`) — custom Caddy pushed, wildcard
> cert issued, three apps live through the full tailscale-auth path. The
> caddy-tailscale placeholder names and the `map {labels.N}` index both
> proved out. Two items remain open below: the tailnet ACL (§3) and
> Litestream (§4's backup half).

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

Set `EDGE_GOARCH` in your `deploy/deploy.env` (created from
[deploy.env.example](deploy.env.example), gitignored) if the edge isn't
amd64. The
push swaps `/usr/bin/caddy` under systemd (the stock unit already grants
`CAP_NET_BIND_SERVICE`, so a plain binary works) and prints the rollback
one-liner.

On the edge host itself, add to Caddy's environment (e.g. a systemd
drop-in):

```text
CLOUDFLARE_API_TOKEN=<token with Zone:DNS:Edit for the domain>
```

And add one line to the main Caddyfile so the generated route file is
actually read (its own header says the same):

```text
import /etc/caddy/bespoke.caddy
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
URL, `BESPOKE_ROOT`, and `BESPOKE_LEMONADE_URL=http://127.0.0.1:13305/api/v1`
for the audio backend — adjust if Lemonade's port differs); units,
binaries, manifests, and litestream config are synced by `bespoke deploy`.

#### Secrets go in `env.d/`, not `env`

Every unit reads `~/bespoke/env`, so **anything in it is in every app's
environment** — including apps installed from someone else's module. Each unit
additionally reads an optional `~/bespoke/env.d/<slug>` that only that unit
sees ([ADR-0032](../docs/adr/0032-app-unit-sandboxing.md)). Deploy creates the
directory `0700`; it is never synced from a dev machine.

Keep only platform-wide values in `~/bespoke/env` (`BESPOKE_BIND_IP`,
`BESPOKE_DOMAIN`, `BESPOKE_LLM_URL`, `BESPOKE_ROOT`, `BESPOKE_LEMONADE_URL`,
`BESPOKE_SPOOL`). Move everything else to the unit that owns it:

```sh
# on the app host, once per secret-owning unit
install -m 600 /dev/null ~/bespoke/env.d/mail
echo 'BESPOKE_MAIL_KEY=…' >> ~/bespoke/env.d/mail
$EDITOR ~/bespoke/env            # delete the line you just moved
systemctl --user restart bespoke-mail
```

Upgrading does not move existing secrets for you. Until you move them, they
stay readable by every app.

#### Migrating to per-app data directories

Hosts created before [ADR-0033](../docs/adr/0033-per-app-data-scope.md) keep
every database in one flat `~/bespoke/data`. App units now scope themselves to
`~/bespoke/data/<slug>`, so the files must move once, with the units stopped:

```sh
cd ~/bespoke
systemctl --user stop 'bespoke-*.service'
tar czf ~/bespoke-data-backup-$(date +%Y%m%d%H%M).tar.gz data   # no backup, no migration
for db in data/*.db; do
  slug=$(basename "$db" .db)
  [ "$slug" = platformd ] && continue   # platformd is not scoped; its database stays at the root
  mkdir -p "data/$slug"
  mv "data/$slug".db* "data/$slug/"
done
mv data/mail-attachments data/mail/ 2>/dev/null   # mail keeps attachments beside its database
```

Secrets move at the same time, out of the shared `env` and into `env.d/`. Note
that the calendar app falls back to `BESPOKE_MAIL_KEY` and mail's Google OAuth
client when its own `BESPOKE_CALENDAR_*` values are unset — if you rely on that
fallback, write the same values into `env.d/calendar` under the `CALENDAR`
names before removing them from the shared file, or calendar loses access to
its stored credentials.

Then deploy from the dev machine and confirm every app comes back:

```sh
just deploy            # creates any missing data dirs, ships hardened units
```

Databases belonging to retired apps stay at the flat path and are ignored;
`platformd.db` deliberately stays at the root.

For the LLM gateway, install the **`copilot` CLI into `~/.local/bin`** on
selfie and sign it in (`copilot`, then authenticate) — the generated
platformd unit puts `~/.local/bin` on PATH for exactly this. Without it
everything else works but chat/summaries are degraded, with a dashboard
warning banner explaining why.

For backups (ADR-0007), install [Litestream](https://litestream.io) into
`~/.local/bin` (no root needed — every Bespoke process is a user unit):

```sh
V=0.5.15
curl -fsSL "https://github.com/benbjohnson/litestream/releases/download/v$V/litestream-$V-linux-x86_64.tar.gz" \
  | tar xz -C /tmp litestream
install -m 755 /tmp/litestream ~/.local/bin/litestream
```

Then write `~/bespoke/env.d/litestream` — **not** the shared file, or every app
can read and delete your backups:

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
# from the dev machine
scp deploy/sudoers/bespoke-edge   <edge>:
scp deploy/sudoers/bespoke-selfie <selfie>:

# then on each host
visudo -cf bespoke-edge   && sudo install -m 0440 bespoke-edge   /etc/sudoers.d/bespoke   # edge
visudo -cf bespoke-selfie && sudo install -m 0440 bespoke-selfie /etc/sudoers.d/bespoke   # selfie
```

If `visudo` is missing (minimal hosts), skip the check and install anyway —
the files are static and were validated elsewhere; a broken sudoers.d file
is ignored by modern sudo rather than locking you out.

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

## Verify (Phase 1 "done when" — closed 2026-08-01)

From any tailnet device: `https://hello.<domain>` renders your Tailscale
login name, and `https://<domain>` shows the dashboard with the Hello app.
(Verified live at `hello.bespoke.ketelsen.cloud`; re-run after any Caddy or
edge change.)

## Troubleshooting

- **502 from Caddy** — unit down? `ssh selfie systemctl --user status
  bespoke-hello`. Or the ACL blocks edge→selfie: test from the edge host with
  `curl http://<selfie-ts-ip>:4101/healthz`.
- **401 "no identity header"** — request reached the app without passing
  `tailscale_auth`; check the caddy-tailscale placeholder names against the
  plugin version (`{http.auth.user.tailscale_login}`).
- **Cert not issued** — check the Cloudflare token scope and `journalctl -u
  caddy` on the edge host for the ACME DNS challenge result.
