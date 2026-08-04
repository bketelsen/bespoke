# 0032 — Split secrets per unit and sandbox app processes

- **Status:** Accepted
- **Date:** 2026-08-04

## Context

[ADR-0031](0031-third-party-app-packages.md) made apps installable from other
people's modules and recorded that the same-UID trust model had become
load-bearing. Two problems were concrete rather than theoretical.

Every generated unit read one shared `~/bespoke/env`. Anything in it was in
every app's environment: the mail app's encryption key, and the object-store
credentials Litestream uses to write — and delete — every backup. An app never
needed to attack anything to hold them.

The units themselves set no sandboxing at all: `EnvironmentFile`, `ExecStart`,
`Restart`. A process could read every sibling's `/proc/<pid>/environ`, share
`/tmp`, open any device node, and consume the host.

These are systemd **user** units. Much of the usual hardening advice assumes a
privileged manager, and one directive is actively misleading here:
`ProtectProc=` configures procfs `hidepid=`, which filters by UID. Every app
runs as the same user, so it would hide nothing between apps.

## Decision

- Each unit reads two environment files: the shared `~/bespoke/env` for
  platform values, then an optional `~/bespoke/env.d/<slug>` for that unit's
  own secrets. `env.d` is created `0700` by deploy and never synced from a
  dev machine. Litestream's credentials and any app secret belong there.
- App, platformd, and Litestream units are sandboxed with the directives that
  work unprivileged: `NoNewPrivileges`, `PrivatePIDs`, `PrivateTmp`,
  `PrivateDevices`, `RestrictNamespaces`, `RestrictAddressFamilies`,
  `RestrictSUIDSGID`, `LockPersonality`, `SystemCallArchitectures`,
  `SystemCallFilter=@system-service`, and `UMask=0077`.
- `PrivatePIDs=yes` is what separates sibling processes — a PID namespace, not
  `hidepid`. A sandboxed app sees one process: itself.
- `RestrictAddressFamilies` allows `AF_INET`, `AF_INET6`, and `AF_UNIX` only.
  `AF_NETLINK` is deliberately excluded, which costs `net.Interfaces()` and
  anything built on it.
- App units get `MemoryHigh=512M`, `MemoryMax=1G`, `TasksMax=128`. platformd
  gets `TasksMax=512` and no memory cap: capping it takes the whole instance
  down, while capping one app takes one app down.
- The deploy watcher is deliberately not sandboxed. It exists to run builds and
  deploys ([ADR-0023](0023-builder-plane-unprivileged-agent-spooled-deploys.md))
  and needs the access a build needs.
- This bounds what one process can reach. It does **not** isolate apps from each
  other's data: databases still share `~/bespoke/data` and a UID, so any app can
  still read any other app's SQLite file. Per-app data scoping and egress
  filtering are not decided here.

## Consequences

- An app secret placed in `env.d/<slug>` is no longer readable by other apps —
  neither from the environment nor through `/proc`.
- Existing hosts keep working: the per-unit file is optional, and secrets
  already in the shared file keep being delivered. Moving them is a manual step
  an owner must take for the split to be worth anything.
- An app that calls `net.Interfaces()` now fails with `netlinkrib: address
  family not supported by protocol`. Nothing in the platform or the reference
  apps does, but the failure is obscure and third-party authors will meet it.
- Anything under `/tmp` is invisible to a unit, including binaries. Bespoke
  paths are unaffected; ad-hoc testing from a temp directory is not.
- The sandbox needs unprivileged user namespaces for its namespacing options.
  Where they are unavailable the units fail to start rather than silently
  running unconfined — loud, but a deploy-time failure.
- The largest remaining exposure is unchanged and now more visible: a hostile
  app can read every database and send it anywhere. That argues for per-app
  UIDs, which is the only way to get per-app egress rules.

## Alternatives considered

- **`ProtectProc=invisible`:** filters by UID; between same-user apps it hides
  nothing. Rejected as security theater.
- **`ProtectSystem=strict` + `ProtectHome=tmpfs` with per-app bind mounts:** the
  isolation that actually matters, but it needs the flat `data/<slug>.db` layout
  to become a directory per app, plus a migration of live databases. Deferred,
  not rejected.
- **`IPAddressDeny=`/`IPAddressAllow=`:** needs the manager to install BPF
  programs, which an unprivileged user manager cannot do. Would appear to work
  and do nothing.
- **`MemoryDenyWriteExecute=yes`:** no benefit for static Go binaries and a
  latent break for anything that ever needs a JIT.

## References

- Shapes: [CLI spec](../specs/bespoke-cli.md),
  [manifest spec](../specs/app-manifest.md),
  [deploy runbook](../../deploy/README.md)
- Builds on: [ADR-0005](0005-process-per-app.md),
  [ADR-0011](0011-split-host-deployment.md),
  [ADR-0031](0031-third-party-app-packages.md)
