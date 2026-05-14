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
- cloudflared installed for Cloudflare Tunnel ingress
- Caddy listening on `127.0.0.1:8080`
- Node.js LTS, Bun, pnpm, PM2
- Litestream binary for SQLite backup workflows
- `/usr/local/bin/simple-vps` for status and route management

Explicit optional installs:

- Install Docker: `simple_vps_install_docker=true` or `--docker`
- Disable Litestream: `simple_vps_install_litestream=false` or `--no-litestream`
- Dev tools / shell / AI CLIs: `simple_vps_install_devtools=true`

Tailscale is on by default. Provide `SIMPLE_VPS_TAILSCALE_AUTH_KEY` or
`--tailscale-auth-key` for unattended login. Public SSH is only removed after
Tailscale is authenticated, so bootstrap runs do not lock themselves out.

Cloudflare Tunnel is installed by default. Provide
`SIMPLE_VPS_CLOUDFLARE_TUNNEL_TOKEN` or `--cloudflare-tunnel-token` to enable
the `cloudflared` service. The tunnel public hostname should route to
`http://127.0.0.1:8080`.

Server-local route management:

```bash
simple-vps status
simple-vps routes
simple-vps publish --host example.com --port 3000
simple-vps unpublish --host example.com
```

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
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests
tests/bootstrap_tarball_smoke.sh
ansible-playbook --syntax-check playbooks/vps-bootstrap.yml
ansible-playbook --syntax-check playbooks/vps-apply.yml
```

CI also runs `shellcheck` and `ansible-lint`.

## License

MIT
