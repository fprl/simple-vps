# Simple VPS

Opinionated production VPS setup for getting apps online quickly and securely.

Goal:

```text
fresh Ubuntu VPS -> run one script -> secure production box ready for apps
```

Simple VPS is intentionally narrow. It is not a VPS framework.

Read [SPEC.md](SPEC.md) for the product direction, security model, architecture,
CLI shape, implementation decisions, and current status.

## Current State

The repo currently uses `install.sh` as the entrypoint and Ansible as the
converge engine.

Default install path:

- Admin user with key-based access
- SSH hardening, UFW, fail2ban, unattended upgrades
- Tailscale installed and started for private admin access
- Caddy listening on `127.0.0.1:8080`
- Node.js LTS, pnpm, PM2

Explicit optional installs:

- Docker: `simple_vps_install_docker=true`
- Dev tools / shell / AI CLIs: `simple_vps_install_devtools=true`

Tailscale is on by default. Provide `SIMPLE_VPS_TAILSCALE_AUTH_KEY` or
`--tailscale-auth-key` for unattended login. Public SSH is only removed after
Tailscale is authenticated, so bootstrap runs do not lock themselves out.

## Quick Start

Remote mode from this checkout:

```bash
./install.sh \
  --mode remote \
  --host 203.0.113.10 \
  --ssh-key ~/.ssh/id_ed25519 \
  --admin-user admin
```

Local mode from this checkout on the VPS:

```bash
./install.sh --mode local --admin-user admin
```

The hosted installer bootstrap path exists, but the final v1 one-liner is still
part of the work tracked in [SPEC.md](SPEC.md).

## Validation

```bash
bash -n install.sh
ansible-playbook --syntax-check playbooks/vps-bootstrap.yml
ansible-playbook --syntax-check playbooks/vps-apply.yml
```

CI also runs `shellcheck` and `ansible-lint`.

## License

MIT
