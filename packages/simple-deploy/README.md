# Simple Deploy

Simple Deploy is the native deploy tool for Simple Stack.

It deploys JS/TS apps from an app repo or CI runner to a hardened VPS prepared
by `simple-vps`. No Docker in v1.

## Shape

```text
local app repo
  -> optional build
  -> package artifact
  -> upload release
  -> install production deps when needed
  -> link shared files
  -> restart systemd services
  -> health check
  -> publish routes
```

Server layout:

```text
/var/apps/my-app
|-- current -> releases/a1b2c3d4...
|-- releases
|   `-- a1b2c3d4...
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
simple-deploy check
simple-deploy check --env production
simple-deploy setup production
simple-deploy deploy production
simple-deploy rollback production
simple-deploy status production
simple-deploy logs production
simple-deploy logs production web
simple-deploy logs production jobs
simple-deploy restart production jobs
simple-deploy env push production .env.production
simple-deploy secret put production API_KEY
simple-deploy secret list production
simple-deploy secret rm production API_KEY
simple-deploy ssh production
```

## Manifest

Simple Deploy reads `simple-deploy.toml` from the app repo root.

```toml
name = "my-app"

[env.production]
server = "admin@100.x.y.z"
path = "/var/apps/my-app"
runtime = "bun"

[services.web]
command = "bun run src/server.ts"
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

`[build]` is optional. No-build Bun and Node apps are first-class. When a build
is declared, `output` is required and only that artifact root is deployed:

```toml
[build]
command = "bun run build"
output = "dist"
include = ["public", "prisma"]
```

## Static App

Serving generated JSON, assets, or static sites should be first-class:

```toml
name = "data-feed"

[build]
command = "bun run generate"
output = "dist"

[env.production]
server = "admin@100.x.y.z"
path = "/var/apps/data-feed"
runtime = "static"

[routes.data]
host = "data.example.com"
type = "static"
```

No systemd service. No port. The route points Caddy at
`/var/apps/data-feed/current`.

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

Simple Deploy calls the Simple VPS server API instead of editing Caddy directly:

```bash
sudo simple-vps app create my-app
sudo simple-vps route proxy app.example.com --port 3000 --app my-app
sudo simple-vps route static data.example.com --root /var/apps/data-feed/current --app data-feed
```

Simple VPS should own generated ingress config, validation, backups, and reloads.
It should not try to wrap all Caddy features. Raw Caddy snippets can be an
escape hatch later.

## GitHub Actions

Store these as repository secrets:

- `SIMPLE_DEPLOY_SSH_KEY`: private key for the deploy SSH user
- `SIMPLE_DEPLOY_KNOWN_HOSTS`: known_hosts entry for the VPS, from `ssh-keyscan -H <host>`

```yaml
name: Deploy

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4
      - uses: oven-sh/setup-bun@v2

      - run: bun install --frozen-lockfile
      - run: bunx simple-deploy deploy production
        env:
          SIMPLE_DEPLOY_SSH_KEY: ${{ secrets.SIMPLE_DEPLOY_SSH_KEY }}
          SIMPLE_DEPLOY_KNOWN_HOSTS: ${{ secrets.SIMPLE_DEPLOY_KNOWN_HOSTS }}
```

When `SIMPLE_DEPLOY_SSH_KEY` is set, Simple Deploy refuses to run without
`SIMPLE_DEPLOY_KNOWN_HOSTS` and uses strict host-key checking for both `ssh` and
`rsync`.

## Non-Goals For V1

- Docker
- dashboard UI
- multi-server orchestration
- Postgres management
- generic plugin system
- full Caddy abstraction
