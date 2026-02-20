# Troubleshooting

## SSH preflight fails in remote mode

- Verify host/IP and reachable port 22.
- Verify bootstrap user (`--bootstrap-user`) is correct.
- Verify key path and permissions (`--ssh-key`).

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
