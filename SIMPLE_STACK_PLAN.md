# Simple Stack Plan

Simple Stack is a local-first production stack for small apps on a hardened VPS.

The point is not to build a platform. The point is to make the boring path
excellent:

```text
secure VPS -> native app releases -> Caddy ingress -> systemd processes
```

## Product Split

```text
simple-vps
  Makes the server boring.

simple-deploy
  Makes native app releases boring.

simple-app
  Optional app templates and personal conventions. Not core for now.
```

`simple-vps` and `simple-deploy` are the real product for now. `simple-app`
already exists elsewhere and should stay personal until the deploy contract is
boring.

## Decisions

- No Docker support in v1 deploys.
- Native systemd services are the default process model.
- JS/Bun/Node is the first-class app stack.
- SQLite is the happy path, with Litestream as the backup primitive.
- Postgres is allowed through `DATABASE_URL`, but Simple Stack does not manage
  Postgres in v1.
- Multiple services per app are required from day one.
- Caddy is managed through simple route primitives plus an escape hatch, not a
  full Caddy wrapper.

## Repository Layout

```text
simple-stack
|-- README.md
|-- SIMPLE_STACK_PLAN.md
|-- install.sh
`-- packages
    |-- simple-vps
    `-- simple-deploy
```

The root `install.sh` remains a compatibility entrypoint for Simple VPS.

## Simple VPS

Simple VPS owns the host contract:

```text
Ubuntu server
admin user
SSH hardening
Tailscale admin access
Cloudflare Tunnel ingress
Caddy bound to 127.0.0.1
Node runtime primitives
Litestream binary
simple-vps CLI
```

Docker is not part of the default host contract. Simple VPS can still install it
behind an explicit `--docker` flag, but Simple Deploy v1 should not use it.

## Simple Deploy

Simple Deploy owns app releases:

```text
/var/apps/my-app
|-- current -> releases/20260514170000
|-- releases
|-- shared
|   |-- .env
|   |-- db
|   |-- logs
|   `-- storage
`-- systemd
```

Deploys run from the local machine:

```text
local build/package
rsync release to server
link shared files
install production dependencies
run migrations when configured
flip current symlink
restart systemd services
run health checks
publish routes
prune old releases
```

## App Manifest

The app repo should contain a deploy manifest:

```toml
app = "my-app"

[production]
server = "admin@100.x.y.z"
path = "/var/apps/my-app"
runtime = "bun"

[services.web]
command = "bun run start"
port = 3000
healthcheck = "/health"

[services.jobs]
command = "bun run plainjob"

[build]
command = "bun install --frozen-lockfile && bun run build"

[routes.app]
host = "app.example.com"
type = "proxy"
service = "web"
```

Static apps are first-class:

```toml
app = "data-feed"

[production]
server = "admin@100.x.y.z"
path = "/var/apps/data-feed"
runtime = "static"

[build]
command = "bun run generate"

[routes.data]
host = "data.example.com"
type = "static"
root = "public"
headers.Cache-Control = "public, max-age=60"
```

## Route Model

Simple VPS should support boring route types:

```bash
simple-vps route proxy app.example.com --port 3000
simple-vps route static data.example.com --root /var/apps/data-feed/current/public
simple-vps route redirect old.example.com --to https://new.example.com
```

It should not expose every Caddy option as CLI flags. Advanced users should get
a raw Caddy snippet escape hatch that is validated before reload.

## Implementation Phases

1. Move Simple VPS into `packages/simple-vps` without breaking it.
2. Add `packages/simple-deploy` with the deploy contract and examples.
3. Add route primitives for proxy/static/redirect.
4. Build Simple Deploy setup/deploy/logs/status/rollback for native Bun/Node
   apps.
5. Add static deploys.
6. Add SQLite/Litestream app examples.
