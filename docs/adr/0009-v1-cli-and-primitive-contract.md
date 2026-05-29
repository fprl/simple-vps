# ADR-0009: V1 CLI And Primitive Contract

- **Status:** Accepted
- **Date:** 2026-05-29
- **Supersedes/amends:** ADR-0006 public CLI commitments, ADR-0007 backup
  command shape, ADR-0008 primitive-freeze details

## Context

ADR-0008 locked the manifest, env-root, runtime identity, static release, and
zero-downtime handoff model. The remaining pre-DX risk is the public CLI and
primitive contract. We still have no users, so this decision intentionally
breaks the current positional-env CLI instead of preserving compatibility.

The CLI should feel repo-centric: a project directory plus `simple-vps.toml`
is the app context. The manifest owns the app name. The env is execution
context and should be passed as a named flag, not as the command subject.

## Decision

### Repo-Centric App Commands

App commands infer the app from the manifest:

```toml
name = "api"
```

The normal deploy flow is:

```bash
simple-vps deploy --env production
```

not:

```bash
simple-vps deploy api --env production
```

The app name must not be duplicated on normal repo-local commands. Outside a
repo or monorepo path, users pass `--config`.

### Required `--env` For Env-Scoped Commands

Env-scoped commands use `--env` / `-e`:

```bash
simple-vps setup --env production
simple-vps deploy --env production
simple-vps status --env production --json
simple-vps logs web --env production --follow --tail 100
simple-vps restart web --env production
simple-vps rollback --env production
simple-vps rollback <release> --env production
simple-vps backup create --env production
simple-vps backup list --env production --json
simple-vps backup rm <backup-id> --env production
simple-vps restore --from <backup-id|path> --env production --dry-run
simple-vps secret set DATABASE_URL --env production
simple-vps secret list --env production --json
simple-vps secret rm DATABASE_URL --env production
simple-vps destroy --env production --confirm api --purge
simple-vps ssh --env production
```

Do not support positional-env aliases such as `simple-vps deploy production`.
Env is execution context, not the command subject. This avoids parser traps if
future commands gain subcommands such as `deploy list`.

`check` may omit `--env` to validate all envs:

```bash
simple-vps check
simple-vps check --env production
```

### `--config`

Repo/app commands accept:

```bash
--config path/to/simple-vps.toml
```

The default is:

```bash
--config ./simple-vps.toml
```

Relative paths inside the manifest resolve relative to the manifest file
directory, not the shell cwd. This includes Dockerfile detection, source
packaging, static `serve` directories, dotenv checks, and generated release
manifest copies.

### Host Commands Stay Host-Centric

Host commands do not need app context:

```bash
simple-vps host status --server deploy@example.com --json
simple-vps host doctor --server deploy@example.com --json
simple-vps host install [flags]
simple-vps app list --server deploy@example.com --json
simple-vps version
```

They inspect or mutate the VPS itself. They should not require a local manifest
unless a future command targets a specific app env.

### Backup Grammar

Use explicit backup subcommands:

```bash
simple-vps backup create --env production [--to path] [--json]
simple-vps backup list --env production [--json]
simple-vps backup rm <backup-id> --env production
```

Do not support `simple-vps backup production`.

### JSON Output Policy

Keep JSON output narrow for v1:

```bash
simple-vps status --env production --json
simple-vps app list --server deploy@example.com --json
simple-vps backup list --env production --json
simple-vps secret list --env production --json
simple-vps backup create --env production --json
```

Do not add JSON output to deploy, restore, destroy, restart, rollback, or logs
for v1. Those are streaming workflows; a proper machine-readable stream would
require a JSONL event system, which is out of scope.

### Route Semantics

Routes keep the ADR-0008 model with these explicit rules:

- exactly one of `process`, `serve`, `redirect`
- `path = "/docs"` matches `/docs`, `/docs/`, and `/docs/page`
- `path = "/docs"` does not match `/docs-v2`
- longest path wins
- duplicate `host + path` is rejected
- `path = "/"` is rejected; omit `path` for root
- static mounted routes strip the mount prefix before file lookup

### TLS Semantics

Supported values:

- omitted / `tls = "auto"`: public automatic HTTPS through Caddy
- `tls = "internal"`: Caddy internal CA for private/smoke hosts

Do not add `tls = "off"` for v1. HTTPS by default is the product line. All
routes on the same host must use one TLS mode.

### Deploy, Destroy, Identity, Rollback

Deploy:

- `deploy --env X` is authoritative for that app/env's generated Caddy routes.
- Removing a route from TOML removes it on the next deploy.
- Deleting `[env.X]` locally does not destroy remote state.

Destroy:

- `destroy --env X --purge` removes app/env runtime state, identity, static
  releases, runtime files, data, and secrets.
- `--purge` does not delete backups. Backup deletion remains explicit.

Identity:

- first setup/deploy writes `simple-vps.json` atomically before runtime
  resources depend on it.
- failed first deploy keeps the minted identity.
- only `destroy --purge` removes the identity anchor.

Rollback:

- re-applies the historical release snapshot: image, manifest, process
  commands, resources, routes, and static assets.
- does not revert `/data`.
- does not revert current secret values.
- does not run `[deploy].release`.

### Resources

Keep:

```toml
resources = { memory = "512m", cpus = 0.5 }
```

`memory` and `cpus` are optional. Validate syntax hard. Catch obvious
host-capacity footguns in preflight checks, but do not turn simple-vps into a
scheduler.

## Consequences

- The public CLI changes before stable v1. This is intentional.
- Tests and docs must move to `--env` / `--config` before DX work.
- `simple-vps init` is DX on top of this contract; it scaffolds only the
  current `--env` / `--config` manifest shape.
- Shell scripts and smoke docs may keep historical commands only where they are
  explicitly historical evidence.

## Required Tests Before DX Work

- `--env` is required on env-scoped commands.
- positional env is rejected.
- `--config` changes the manifest/app root.
- manifest-relative paths are used for Dockerfile, source archive, static
  `serve` dirs, and dotenv checks.
- route exact-or-child matching: `/docs` does not match `/docs-v2`.
- longest path wins.
- static prefix stripping.
- duplicate route rejection.
- `path = "/"` rejection.
- TLS consistency per host.
- deploy removes omitted routes.
- deleting local env config does not destroy remote state.
- purge preserves backups.
- first-deploy identity stability.
- rollback snapshot behavior.
- rollback does not run `[deploy].release`.
