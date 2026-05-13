# OpenVPS

Opinionated production VPS setup for getting apps online quickly and securely.

OpenVPS is not trying to become a generic VPS framework. The goal is simpler:

```text
fresh Ubuntu VPS -> run one script -> secure production box ready for apps
```

The current implementation uses Ansible as the converge engine and `install.sh` as
the user-facing entrypoint. The next version should become more opinionated:
Tailscale for private admin access, Cloudflare Tunnel for public ingress, Caddy as
a local-only reverse proxy, PM2 as the default Node app runner, and a small
`openvps` CLI for day-2 operations.

## Product Direction

### Production Default

The default install should create a secure production baseline:

- Admin user with key-based access
- SSH hardened and reachable only through Tailscale
- UFW deny-all inbound by default
- Optional Hetzner firewall automation, also deny-all inbound
- Tailscale installed for private admin access
- Cloudflare Tunnel installed for public web ingress
- Caddy listening locally, not exposed directly to the internet
- Node.js LTS + pnpm
- PM2 as the default process manager for Node apps
- Basic server packages: `git`, `curl`, `jq`, `htop`, `tmux`, `rsync`, `unzip`, `ncdu`
- `/usr/local/bin/openvps` installed for server-local management

Public traffic should flow like this:

```text
Browser
  -> Cloudflare
  -> Cloudflare Tunnel
  -> Caddy on 127.0.0.1
  -> PM2 app on 127.0.0.1
```

Admin traffic should flow like this:

```text
Laptop
  -> Tailscale
  -> VPS SSH
```

The VPS should not expose public `22`, `80`, `443`, or random app ports.

### What Stays Optional

Personal comfort tools should not be able to break the production baseline. They
belong behind the CLI, not in the default install:

```bash
openvps devtools install
```

That can install the shell and agent setup:

- Zsh + Oh My Zsh
- Powerlevel10k
- Zsh autosuggestions/highlighting
- `fzf`, `zoxide`, `atuin`, `lsd`, `bat`
- AI CLIs: Codex, Claude, Gemini, OpenCode

Docker should be optional too:

```bash
openvps docker install
```

### Day-2 CLI

The CLI should stay tiny and boring:

```bash
openvps status
openvps publish --host example.com --port 3000
openvps unpublish --host example.com
openvps routes
openvps devtools install
```

`publish` means: expose this local service through the production ingress stack.

For example:

```bash
openvps publish --host example.com --port 3000
openvps publish --host api.example.com --port 8080
```

Internally, OpenVPS should maintain one source of truth:

```text
/etc/openvps/state.json
```

Then generate:

```text
/etc/cloudflared/config.yml
/etc/caddy/Caddyfile
```

The command should be idempotent and defensive:

- Existing matching route: no-op
- Existing conflicting route: fail unless `--force`
- Existing Caddy host: fail unless `--replace`
- Validate Caddy before reload
- Validate Cloudflare/cloudflared config before reload where possible
- Keep backups before changing generated files

### Cloudflare Model

One Cloudflare Tunnel per server:

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

Cloudflare dashboard setup should be one-time only:

- Domain on Cloudflare DNS
- Cloudflare account secured with passkeys/2FA
- API token for OpenVPS automation
- Optional Cloudflare Zero Trust/Access policies for private dashboards

Per-server setup should be automated by OpenVPS.

## Implementation Plan

1. Make the hosted installer real.
   - `curl .../install.sh | bash` must work without a local checkout.
   - The bootstrap script should download a pinned repo release/tarball, then run the real installer from that extracted directory.

2. Simplify the default install.
   - Keep production essentials only.
   - Include Node.js, pnpm, and PM2 because they are the default app runtime.
   - Move terminal comfort tools, AI CLIs, and Docker out of the default role.
   - Keep dev tooling available through `openvps devtools install`.

3. Add the secure ingress baseline.
   - Install Tailscale.
   - Install Cloudflare Tunnel.
   - Configure Caddy local-only.
   - Set UFW to deny inbound by default.
   - Allow SSH only through Tailscale.

4. Add the server-local CLI.
   - Start as a small Python or Bash script at `/usr/local/bin/openvps`.
   - Use `/etc/openvps/state.json` as the source of truth.
   - Support `status`, `routes`, `publish`, and `unpublish`.

5. Automate app publishing safely.
   - Create/update Cloudflare DNS/tunnel routes through API tokens.
   - Generate Caddy and cloudflared config.
   - Validate before reload.
   - Fail on conflicts unless explicitly forced.

6. Add real validation.
   - Syntax checks for shell and Ansible.
   - Caddy config validation.
   - Installer smoke test on fresh Ubuntu 24.04.
   - Idempotency test: run installer twice and verify low/no drift.

7. Only then polish docs and release.
   - Document the exact expected server state.
   - Document recovery paths when Cloudflare/Tailscale breaks.
   - Tag a v1 once the fresh Hetzner flow is boring.

## Current Implementation

This section describes the code that exists today, before the refactor described
above.

A fresh Ubuntu VPS becomes a productive development environment with:

- **Security**: UFW firewall, fail2ban, SSH hardening, unattended upgrades
- **Container Runtime**: Docker + Docker Compose plugin
- **Web Server**: Caddy with automatic HTTPS support
- **Shell UX**: Zsh + Oh My Zsh + Powerlevel10k + modern CLI tooling
- **Languages**: Node.js LTS, Bun, Python (`uv`), Go, Rust
- **AI CLIs**: Claude Code, OpenAI Codex, Google Gemini CLI, OpenCode

The plan is to make Node.js, pnpm, and PM2 the default runtime, then move shell
comfort tools, AI CLIs, and Docker behind explicit CLI commands so they cannot
break the default production install.

## Current Quick Start

OpenVPS supports two flows with the same installer interface.

### Option A: Run from your local machine (remote VPS)

From this repository:

```bash
./install.sh \
  --mode remote \
  --host 203.0.113.10 \
  --ssh-key ~/.ssh/id_ed25519 \
  --admin-user admin
```

### Option B: Run directly on the VPS from a checkout

```bash
./install.sh --mode local --admin-user admin
```

The hosted installer is part of the plan above. Today, `install.sh` expects the
repo playbooks and roles to exist beside it.

### Guided Terminal Wizard

For a full interactive setup flow:

```bash
./install.sh --interactive
```

If you run `./install.sh` with no arguments in an interactive terminal, the wizard starts automatically.

## Password-Only VPS Credentials

Some VPS providers initially give only `root + password`.

- `remote` mode expects SSH key-based auth.
- If you only have password auth, first SSH into the VPS and run `local` mode there.

Safe first-run pattern for password-only access:

1. SSH into VPS with password as root.
2. Add your public key to `/root/.ssh/authorized_keys` or pass `--ssh-public-key-file`.
3. Run OpenVPS locally on the VPS:

```bash
./install.sh --mode local --admin-user admin
```

OpenVPS will block local mode if no key source is available, to avoid lockout after SSH hardening.

## Installer Options

```text
--mode <local|remote|auto>
--host <ip-or-hostname>        # remote mode target
--bootstrap-user <user>        # default: root
--ssh-key <path>
--ssh-public-key-file <path>
--admin-user <name>            # default: admin
--tailscale / --no-tailscale
--check
--interactive / --no-interactive
--yes
```

## Prerequisites

- Fresh Ubuntu 22.04 or 24.04 VPS
- Root SSH access for initial bootstrap
- SSH key-based authentication recommended

## Installation Model

OpenVPS converges in two phases:

1. **Bootstrap** (`playbooks/vps-bootstrap.yml`)
- Creates/configures admin user
- Sets baseline system state

2. **Apply** (`playbooks/vps-apply.yml`)
- Applies security hardening
- Installs Docker, Caddy, developer tooling, and AI CLIs in the current implementation

`install.sh` orchestrates both phases with shared variables.

## Scope (Current)

OpenVPS currently focuses on a strong default base:

- Secure baseline
- Developer-ready runtime
- Idempotent reruns

Advanced extension mechanics (for example custom role injection) are intentionally deferred until the base flow is stable.

## Docs

- `docs/local-mode.md`
- `docs/remote-mode.md`
- `docs/security-modes.md`
- `docs/troubleshooting.md`
- `docs/server-audit.md`
- `docs/plan.md`

## From Source (Direct Ansible)

```bash
ansible-playbook -i inventory/hosts.ini playbooks/vps-bootstrap.yml \
  -e "target=vps openvps_admin_user=admin"

ansible-playbook -i inventory/hosts.ini playbooks/vps-apply.yml \
  -e "target=vps openvps_admin_user=admin"
```

## Tailscale (Optional)

Tailscale is opt-in:

```bash
./install.sh --mode remote --host 203.0.113.10 --tailscale
```

Or with direct Ansible:

```bash
ansible-playbook -i inventory/hosts.ini playbooks/vps-apply.yml \
  -e "target=vps security_enable_tailscale=true"
```

## Why Ansible?

- **Idempotent**: safe to rerun
- **Declarative**: desired state over ad-hoc shell logic
- **Auditable**: explicit role/task structure
- **Composable**: straightforward path to future profiles/extensions

## License

MIT

## Contributing

PRs are welcome. Current priorities:

- hardening the base installer flow
- improving idempotency and tests
- documentation accuracy vs runtime behavior

See `CONTRIBUTING.md` for validation and test matrix details.
