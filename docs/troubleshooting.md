# Troubleshooting

## SSH preflight fails in remote mode

- Verify host/IP and reachable port 22.
- Verify bootstrap user (`--bootstrap-user`) is correct.
- Verify key path and permissions (`--ssh-key`).
- Remote mode does not use password prompts. For password-only credentials, use local mode on the VPS first.

## Provider gave only password credentials

Use this bootstrapping sequence:

1. SSH into the VPS as root using password.
2. Add your SSH public key to `/root/.ssh/authorized_keys` (or prepare a public key file).
3. Run:

```bash
./install.sh --mode local --admin-user admin
```

OpenVPS prevents local-mode execution when no key source is available, to avoid lockout after SSH hardening.

## Apply fails as admin user

- Installer retries apply with bootstrap user automatically.
- Check whether admin SSH keys were provisioned.
- Provide explicit key with `--ssh-public-key-file` if needed.

## Ansible permission/temp errors locally

If local ansible temp dir is restricted, run with:

```bash
ANSIBLE_LOCAL_TEMP="$PWD/.ansible/tmp" ./install.sh ...
```

## Tailscale not installed

- Ensure `--tailscale` was set (or `security_enable_tailscale=true` in Ansible).
- Re-run apply phase after network/transient apt issues are resolved.
