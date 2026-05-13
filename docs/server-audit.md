# Server Audit Checklist

Use this after a successful run to verify expected state.

## Access and Security

```bash
sudo grep -E '^(PermitRootLogin|PasswordAuthentication|PubkeyAuthentication|X11Forwarding|MaxAuthTries)' /etc/ssh/sshd_config
sudo ufw status verbose
sudo systemctl is-active fail2ban unattended-upgrades
```

Expected:

- `PermitRootLogin prohibit-password`
- `PasswordAuthentication no`
- UFW enabled with `22/tcp`, `80/tcp`, `443/tcp`
- `fail2ban` active

## Core Services

```bash
sudo systemctl is-active caddy
caddy version
node -v
pnpm -v
pm2 -v
```

Expected:

- Caddy active
- Node.js, pnpm, and PM2 installed

## Optional Docker Runtime

Docker is part of the current implementation, but the target production runtime is
Node.js + PM2 with Docker available as an explicit optional install.

```bash
sudo systemctl is-active docker
docker --version
```

Expected when Docker is installed:

- Docker active

## Developer Toolchain

```bash
zsh --version
bun --version
uv --version
go version
rustc --version
```

Expected:

- Commands resolve and return versions

## Agent CLIs

```bash
codex --version || npx @openai/codex --version
gemini --version || npx @google/gemini-cli --version
opencode --version
claude --version
```

Expected:

- CLIs installed and invokable

## Idempotency Sanity Check

Re-run apply path and verify low/no drift:

```bash
./install.sh --mode remote --host <VPS_IP> --bootstrap-user root --ssh-key <KEY_PATH>
```

Expected:

- No failures
- Minimal `changed` tasks
