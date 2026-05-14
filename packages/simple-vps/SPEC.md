# Simple VPS Spec

This is the source of truth for the project. Keep this file updated before
spreading details into code comments, README snippets, or release notes.

## Goal

Simple VPS should turn a fresh Ubuntu VPS into a secure production app host:

```text
fresh Ubuntu VPS -> run one script -> secure production box ready for apps
```

The tool is opinionated. It should optimize for shipping small production apps
quickly and safely, not for becoming a generic server framework.

## Non-Goals

- No Kubernetes.
- No plugin marketplace.
- No multi-provider abstraction.
- No dashboard UI.
- No generic profile system until the base flow is boring.
- No Brewfile-based production server setup.

## Target Production Architecture

Public traffic:

```text
Browser
  -> Cloudflare
  -> Cloudflare Tunnel
  -> local ingress on 127.0.0.1
  -> app runtime on 127.0.0.1
```

Admin traffic:

```text
Laptop
  -> Tailscale
  -> VPS SSH
```

The VPS should not expose public `22`, `80`, `443`, or random app ports.

Default local ingress is Caddy on `127.0.0.1:8080`. Container-based deploy
tools are outside the v1 product direction, but Simple VPS should still prepare
the box and keep public ports closed.

Simple VPS owns server readiness. App repositories own app deployment,
environment variables, migrations, app-specific users, processes, and database
backup configuration.

## Default Install

The default install should create:

- Admin user with key-based access
- SSH hardened and reachable only through Tailscale after bootstrap
- UFW deny-all inbound by default
- Optional Hetzner firewall automation, also deny-all inbound
- Tailscale for private admin access
- Cloudflare Tunnel for public web ingress
- Caddy listening locally, not on the public interface
- Node.js LTS
- Bun
- pnpm
- PM2
- Litestream binary for SQLite backup workflows
- `/usr/local/bin/simple-vps`
- `/etc/simple-vps/state.json`
- Basic server packages: `git`, `curl`, `jq`, `htop`, `tmux`, `rsync`, `unzip`, `ncdu`
- `/usr/local/bin/simple-vps` for server-local management

## Optional Installs

Personal comfort tools should not be able to break the production baseline.

Docker is optional and off by default:

```bash
./install.sh --docker
```

Simple VPS does not require apps to use Docker.

Dev tools:

```bash
simple-vps devtools install
```

This can install:

- Zsh + Oh My Zsh
- Powerlevel10k
- Zsh autosuggestions/highlighting
- `fzf`, `zoxide`, `atuin`, `lsd`, `bat`
- uv, Go, Rust
- AI CLIs: Codex, Claude, Gemini, OpenCode

Litestream is installed by default because SQLite backup/restore is part of the
intended production story. Simple VPS installs the binary, but app repositories
own the database path, replica credentials, and systemd service configuration.

## CLI Shape

Keep the CLI tiny:

```bash
simple-vps status
simple-vps route list
simple-vps route list --json
simple-vps route proxy example.com --port 3000
simple-vps route static data.example.com --root /var/apps/data/current/public
simple-vps route redirect old.example.com --to https://new.example.com
simple-vps route remove example.com
simple-vps devtools install
```

`route proxy` means expose a local service through the production ingress stack.

Examples:

```bash
simple-vps route proxy example.com --port 3000
simple-vps route proxy api.example.com --port 8080
simple-vps route static data.example.com --root /var/apps/data-feed/current/public
```

`publish`, `unpublish`, and `routes` remain compatibility aliases for simple
proxy routes.

## Routing State

Simple VPS should maintain one source of truth on the server:

```text
/etc/simple-vps/state.json
```

Generated files:

```text
/etc/caddy/Caddyfile
/etc/caddy/simple-vps/routes.caddy
/etc/cloudflared/config.yml
```

Rules:

- Existing matching route: no-op
- Existing conflicting route: fail unless `--force`
- Existing Caddy host: fail unless `--force`
- Validate Caddy before reload
- Validate Cloudflare/cloudflared config before reload where possible
- Keep backups before changing generated files
- Preserve user-owned Caddy snippets under `/etc/caddy/conf.d`
- Detect manual edits in Simple VPS generated Caddy files and fail unless
  `--force`

## Cloudflare Model

Use one Cloudflare Tunnel per server:

```text
prod-1 tunnel -> apps on prod-1
prod-2 tunnel -> apps on prod-2
staging-1 tunnel -> apps on staging-1
```

Many domains can live on the same server:

```text
example.com        -> prod-1 tunnel -> Caddy -> app on :3000
www.example.com    -> prod-1 tunnel -> Caddy -> app on :3000
anotherapp.com     -> prod-1 tunnel -> Caddy -> app on :3001
api.anotherapp.com -> prod-1 tunnel -> Caddy -> app on :3002
```

One-time Cloudflare setup:

- Domain on Cloudflare DNS
- Cloudflare account secured with passkeys/2FA
- API token for Simple VPS automation
- Optional Cloudflare Zero Trust/Access policies for private dashboards

Per-server Cloudflare setup should be automated by Simple VPS.

## Installer Model

`install.sh` should be the public entrypoint.

Target one-liner:

```bash
curl -fsSL https://simple-vps.dev/install.sh | bash
```

The hosted script should download a pinned release/tarball, then run the real
installer from that extracted checkout.

Current implementation already has a bootstrap-download path, but v1 still needs
the full production flow validated on a fresh VPS.

## Ansible Decision

Use Ansible for now.

Why:

- It is already in the repo.
- It gives idempotent reruns.
- It is good at users, packages, services, files, handlers, and system config.
- The existing playbooks already syntax-check.

Boundaries:

- Do not turn this into a generic Ansible framework.
- Do not add roles/profiles unless they remove immediate complexity.
- Keep `install.sh` as the only user-facing install entrypoint.

Why not Brewfile:

- Brewfile is good for local developer machines.
- It is not the right tool for Ubuntu production server state.
- It does not own SSH hardening, UFW, users, systemd units, Cloudflare Tunnel
  config, Tailscale service setup, or rollback/validation semantics.

If Ansible starts slowing down the one-script path, port the narrow role logic to
a small Bash/Python installer later. Do not restart from scratch just to change
the engine.

## Current Implementation

Current default apply path installs:

- System essentials
- Admin user
- Security baseline
- Tailscale package and `tailscaled`
- `cloudflared` package for Cloudflare Tunnel ingress
- Caddy local-only
- `/etc/caddy/simple-vps` for generated Caddy snippets
- `/etc/caddy/conf.d` for user-owned Caddy snippets
- Node.js LTS
- Bun
- pnpm
- PM2
- Litestream

Current optional variables:

- `simple_vps_install_docker=true` or `--docker` to install Docker
- `simple_vps_install_litestream=false` or `--no-litestream` to disable
  Litestream binary installation
- `simple_vps_install_devtools=true`
- `security_enable_tailscale=false` or `--no-tailscale` to disable Tailscale
- `simple_vps_enable_cloudflare_tunnel=false` or `--no-cloudflare-tunnel`
  to disable Cloudflare Tunnel setup

Current Tailscale behavior:

- Tailscale is enabled by default.
- `SIMPLE_VPS_TAILSCALE_AUTH_KEY` or `--tailscale-auth-key` enables
  non-interactive `tailscale up`.
- Public SSH remains allowed until the server has a Tailscale IP.
- Once Tailscale is authenticated, UFW allows SSH on `tailscale0` and removes
  the public SSH allow rule.

Current Cloudflare Tunnel behavior:

- `cloudflared` is installed by default from Cloudflare's apt repository.
- `SIMPLE_VPS_CLOUDFLARE_TUNNEL_TOKEN` or `--cloudflare-tunnel-token` enables
  a remotely-managed tunnel service using `/etc/cloudflared/tunnel-token`.
- `SIMPLE_VPS_CLOUDFLARE_TUNNEL_CONFIG` or `--cloudflare-tunnel-config`
  enables a service using an existing local `cloudflared` config path.
- If neither token nor config path is provided, `cloudflared` is installed but
  the service is not enabled.
- Until Cloudflare API automation lands, configure the tunnel public hostname in
  Cloudflare Zero Trust to route to `http://127.0.0.1:8080`.

Current CLI behavior:

- `simple-vps status` prints state path, route count, service status, and
  installed tool status for runtime primitives.
- `simple-vps route list` lists routes from state.
- `simple-vps route list --json` emits route state as JSON.
- `simple-vps route proxy HOST --port PORT` writes a proxy route and regenerates
  managed Caddy files.
- `simple-vps route static HOST --root PATH` writes a static file route and
  regenerates managed Caddy files.
- `simple-vps route redirect HOST --to URL` writes a redirect route and
  regenerates managed Caddy files.
- `simple-vps route remove HOST` removes a route and regenerates managed Caddy
  files.
- `simple-vps publish`, `simple-vps unpublish`, and `simple-vps routes` remain
  compatibility aliases.
- `simple-vps generate-caddy` regenerates managed Caddy files from state.
- Mutating commands require root, validate the generated Caddyfile, keep
  backups under `/etc/simple-vps/backups`, and reload Caddy.

Known gaps:

- Cloudflare Tunnel API automation is not implemented yet.
- Cloudflare config generation is not implemented yet.
- Hosted installer needs fresh-VPS validation.
- Public SSH is still needed during bootstrap unless Tailscale auth succeeds.
- Static inventory/direct Ansible path is legacy and should not drive the product.

## Implementation Plan

1. Keep the README short and this spec authoritative.
2. Make the hosted one-liner real and tested.
3. Make Tailscale part of the secure baseline.
4. Add Cloudflare Tunnel install and service setup.
5. Add `/usr/local/bin/simple-vps`.
6. Add Cloudflare API/config automation for tunnel public hostnames.
7. Add fresh Ubuntu 24.04 smoke testing and idempotency testing.
8. Only then tag v1.

## Validation

Local checks:

```bash
bash -n install.sh
PYTHONDONTWRITEBYTECODE=1 python3 -m py_compile roles/infra/files/simple-vps
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests
tests/bootstrap_tarball_smoke.sh
ansible-playbook --syntax-check playbooks/vps-bootstrap.yml
ansible-playbook --syntax-check playbooks/vps-apply.yml
```

CI should also run:

```bash
shellcheck install.sh
ansible-lint playbooks/vps-bootstrap.yml playbooks/vps-apply.yml
```
