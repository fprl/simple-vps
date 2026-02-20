# Local Mode Quickstart

Use local mode when you are already on the target VPS.

## Requirements

- Ubuntu 22.04 or 24.04
- Root shell

## Command

```bash
./install.sh --mode local --admin-user admin
```

## Optional Flags

- `--tailscale`
- `--timezone UTC`
- `--locale en_US.UTF-8`
- `--check`

## Notes

- If Ansible is missing, installer attempts `apt-get install ansible`.
- Local mode uses `localhost` with Ansible local connection.
