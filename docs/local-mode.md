# Local Mode Quickstart

Use local mode when you are already on the target VPS.

You can also start the guided wizard and choose local mode in-menu:

```bash
./install.sh --interactive
```

## Requirements

- Ubuntu 22.04 or 24.04
- Root shell

## Command

```bash
./install.sh --mode local --admin-user admin
```

## Optional Flags

- `--tailscale`
- `--check`

## Notes

- If Ansible is missing, installer attempts `apt-get install ansible`.
- Local mode uses `localhost` with Ansible local connection.
