# Remote Mode Quickstart

Use remote mode when running from your laptop/workstation against a VPS target.

You can also start the guided wizard and choose remote mode in-menu:

```bash
./install.sh --interactive
```

## Requirements

- Ansible installed locally
- SSH access to the VPS bootstrap user (usually `root`)
- A private key authorized on the VPS

## Command

```bash
./install.sh \
  --mode remote \
  --host 203.0.113.10 \
  --ssh-key ~/.ssh/id_ed25519 \
  --admin-user admin
```

## Optional Flags

- `--tailscale` to enable Tailscale setup
- `--timezone UTC`
- `--check` for dry-run
- `--yes` for non-interactive runs

## Notes

- The installer runs bootstrap first, then apply.
- If apply cannot connect as admin yet, it retries apply with the bootstrap user.
