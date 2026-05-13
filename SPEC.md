# OpenVPS Spec

This is the source of truth for the project. Keep this file updated before
spreading details into code comments, README snippets, or release notes.

## Goal

OpenVPS should turn a fresh Ubuntu VPS into a secure production app host:

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
  -> Caddy on 127.0.0.1
  -> PM2 app on 127.0.0.1
```

Admin traffic:

```text
Laptop
  -> Tailscale
  -> VPS SSH
```

The VPS should not expose public `22`, `80`, `443`, or random app ports.

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
- pnpm
- PM2
- Basic server packages: `git`, `curl`, `jq`, `htop`, `tmux`, `rsync`, `unzip`, `ncdu`
- `/usr/local/bin/openvps` for server-local management

## Optional Installs

Personal comfort tools should not be able to break the production baseline.

Dev tools:

```bash
openvps devtools install
```

This can install:

- Zsh + Oh My Zsh
- Powerlevel10k
- Zsh autosuggestions/highlighting
- `fzf`, `zoxide`, `atuin`, `lsd`, `bat`
- Bun, uv, Go, Rust
- AI CLIs: Codex, Claude, Gemini, OpenCode

Docker:

```bash
openvps docker install
```

Docker is useful, but it is not the default runtime for this tool.

## CLI Shape

Keep the CLI tiny:

```bash
openvps status
openvps publish --host example.com --port 3000
openvps unpublish --host example.com
openvps routes
openvps devtools install
openvps docker install
```

`publish` means expose a local service through the production ingress stack.

Examples:

```bash
openvps publish --host example.com --port 3000
openvps publish --host api.example.com --port 8080
```

## Routing State

OpenVPS should maintain one source of truth on the server:

```text
/etc/openvps/state.json
```

Generated files:

```text
/etc/cloudflared/config.yml
/etc/caddy/Caddyfile
```

Rules:

- Existing matching route: no-op
- Existing conflicting route: fail unless `--force`
- Existing Caddy host: fail unless `--replace`
- Validate Caddy before reload
- Validate Cloudflare/cloudflared config before reload where possible
- Keep backups before changing generated files

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
- API token for OpenVPS automation
- Optional Cloudflare Zero Trust/Access policies for private dashboards

Per-server Cloudflare setup should be automated by OpenVPS.

## Installer Model

`install.sh` should be the public entrypoint.

Target one-liner:

```bash
curl -fsSL https://openvps.dev/install.sh | bash
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
- Caddy local-only
- Node.js LTS
- pnpm
- PM2

Current optional variables:

- `openvps_install_docker=true`
- `openvps_install_devtools=true`
- `security_enable_tailscale=true`

Known gaps:

- Tailscale is still opt-in; target is default private admin access.
- Cloudflare Tunnel is not implemented yet.
- `/usr/local/bin/openvps` CLI is not implemented yet.
- Hosted installer needs fresh-VPS validation.
- Public SSH is still needed during bootstrap.
- Static inventory/direct Ansible path is legacy and should not drive the product.

## Implementation Plan

1. Keep the README short and this spec authoritative.
2. Make the hosted one-liner real and tested.
3. Make Tailscale part of the secure baseline.
4. Add Cloudflare Tunnel install and service setup.
5. Add `/usr/local/bin/openvps`.
6. Add `openvps publish` / `unpublish` with generated Caddy/cloudflared config.
7. Add fresh Ubuntu 24.04 smoke testing and idempotency testing.
8. Only then tag v1.

## Validation

Local checks:

```bash
bash -n install.sh
ansible-playbook --syntax-check playbooks/vps-bootstrap.yml
ansible-playbook --syntax-check playbooks/vps-apply.yml
```

CI should also run:

```bash
shellcheck install.sh
ansible-lint playbooks/vps-bootstrap.yml playbooks/vps-apply.yml
```
