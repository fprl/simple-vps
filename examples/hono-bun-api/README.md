# Hono Bun API

Minimal container app example.

Before deploying, edit `simple-vps.toml`:

- set `[env.production].server`
- set `[routes.app].host`

```bash
simple-vps check --env production
simple-vps setup --env production
simple-vps deploy --env production
curl https://api.example.com/health
```
