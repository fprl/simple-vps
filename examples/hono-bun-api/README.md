# Hono Bun API

Minimal container app example.

Before deploying, edit `simple-vps.toml`:

- set `[env.production].server`
- set `[routes.app].host`

```bash
simple-vps check production
simple-vps setup production
simple-vps deploy production
curl https://api.example.com/health
```
