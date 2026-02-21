# OpenVPS

One-command provisioning for developer-ready VPS instances.

OpenVPS gives you a secure, modern baseline on a fresh Ubuntu VPS, with Ansible as the converge engine and `install.sh` as the user-facing entrypoint.

## What You Get

A fresh Ubuntu VPS becomes a productive development environment with:

- **Security**: UFW firewall, fail2ban, SSH hardening, unattended upgrades
- **Container Runtime**: Docker + Docker Compose plugin
- **Web Server**: Caddy with automatic HTTPS support
- **Shell UX**: Zsh + Oh My Zsh + Powerlevel10k + modern CLI tooling
- **Languages**: Node.js LTS, Bun, Python (`uv`), Go, Rust
- **AI CLIs**: Claude Code, OpenAI Codex, Google Gemini CLI, OpenCode

## Quick Start

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

### Option B: Run directly on the VPS (local mode)

If using hosted installer:

```bash
curl -fsSL https://openvps.dev/install.sh | bash -s -- --mode local --admin-user admin
```

If running from a local checkout on the VPS:

```bash
./install.sh --mode local --admin-user admin
```

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
- Installs Docker, Caddy, developer tooling, and AI CLIs

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
