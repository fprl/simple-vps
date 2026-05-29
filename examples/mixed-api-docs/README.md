# Mixed API And Docs

Container app with a static `/docs` route in the same release.

Before deploying, edit `simple-vps.toml`:

- set `[env.production].server`
- set both route hosts

```bash
simple-vps check production
simple-vps setup production
simple-vps deploy production
curl https://mixed.example.com/health
curl https://mixed.example.com/docs
```

Rollback and restore move the web process and `docs-dist/` snapshot together.
