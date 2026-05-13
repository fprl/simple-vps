# Contributing

Thanks for helping improve Simple VPS.

## Local Validation Before PR

Run these checks from repo root:

```bash
bash -n install.sh
ansible-playbook --syntax-check playbooks/vps-bootstrap.yml
ansible-playbook --syntax-check playbooks/vps-apply.yml
ansible-lint playbooks/vps-bootstrap.yml playbooks/vps-apply.yml
```

## Manual Test Matrix

For installer or role changes, validate both paths:

1. Existing VPS (compatibility + idempotency)
2. Fresh VPS (true bootstrap)

Recommended remote-mode test sequence:

```bash
./install.sh --mode remote --host <VPS_IP> --bootstrap-user root --ssh-key <KEY_PATH> --check
./install.sh --mode remote --host <VPS_IP> --bootstrap-user root --ssh-key <KEY_PATH>
./install.sh --mode remote --host <VPS_IP> --bootstrap-user root --ssh-key <KEY_PATH>
```

## Password-Only Providers

If provider gives only password credentials first:

1. SSH into VPS as root with password.
2. Add public key to `/root/.ssh/authorized_keys` (or prepare `--ssh-public-key-file`).
3. Run local mode on the VPS:

```bash
./install.sh --mode local --admin-user admin
```

Simple VPS intentionally blocks local mode when no key source exists to avoid lockout after SSH hardening.

## PR Focus

Please prioritize:

- Idempotency
- Reproducibility
- Clear errors and recovery paths
- Keeping `SPEC.md` accurate
