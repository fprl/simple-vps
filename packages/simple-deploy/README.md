# Simple Deploy

Simple Deploy is the planned native deploy tool for Simple Stack.

It deploys apps from your local machine to a hardened VPS prepared by
`simple-vps`. No Docker in v1.

## Shape

```text
local app repo
  -> build/package
  -> upload release
  -> link shared files
  -> restart systemd services
  -> health check
  -> publish routes
```

Server layout:

```text
/var/apps/my-app
|-- current -> releases/20260514170000
|-- releases
|   `-- 20260514170000
|-- shared
|   |-- .env
|   |-- db
|   |-- logs
|   `-- storage
`-- systemd
```

## Commands

Target CLI shape:

```bash
simple-deploy init
simple-deploy setup production
simple-deploy deploy production
simple-deploy rollback production
simple-deploy status production
simple-deploy logs production
simple-deploy logs production web
simple-deploy logs production jobs
simple-deploy restart production jobs
simple-deploy env push production .env.production
simple-deploy ssh production
```

## Manifest

Simple Deploy reads `simple-deploy.toml` from the app repo root.

```toml
app = "my-app"

[production]
server = "admin@100.x.y.z"
path = "/var/apps/my-app"
runtime = "bun"

[build]
command = "bun install --frozen-lockfile && bun run build"

[services.web]
command = "bun run start"
port = 3000
healthcheck = "/health"

[services.jobs]
command = "bun run plainjob"

[routes.app]
host = "app.example.com"
type = "proxy"
service = "web"
```

This creates systemd services like:

```text
simple-my-app-web.service
simple-my-app-jobs.service
```

Only services with ports get routed.

## Static App

Serving generated JSON, assets, or static sites should be first-class:

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

No systemd service. No port. The route points Caddy at
`/var/apps/data-feed/current/public`.

## Databases

SQLite is the happy path:

```text
/var/apps/my-app/shared/db/app.sqlite
```

Litestream backup configuration belongs to the app. Simple Deploy can generate
or manage that later, but v1 should not become a database platform.

Postgres works through `DATABASE_URL`. Simple Deploy should not install or
operate Postgres in v1.

## Route Contract

Simple Deploy should call Simple VPS route primitives instead of editing Caddy
directly:

```bash
simple-vps route proxy app.example.com --port 3000
simple-vps route static data.example.com --root /var/apps/data-feed/current/public
```

Simple VPS should own generated ingress config, validation, backups, and reloads.
It should not try to wrap all Caddy features. Raw Caddy snippets can be an
escape hatch later.

## Non-Goals For V1

- Docker
- dashboard UI
- multi-server orchestration
- Postgres management
- generic plugin system
- full Caddy abstraction
