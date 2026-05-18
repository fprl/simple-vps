# Simple VPS

Simple VPS is one CLI for running JS/TS apps on your own VPS without Docker.

```text
fresh Ubuntu VPS  ->  install.sh         ->  hardened box
your app repo     ->  simple-vps deploy  ->  live app
```

## Packages

```text
packages/simple-vps
  Host installer, Ansible roles, and privileged server-side helper.

packages/cli
  Public Bun CLI for app deploys and app operations.
```

## Start Here

The public product contract lives in [SPEC.md](SPEC.md).
The host security model lives in [docs/security-model.md](docs/security-model.md).

The root installer delegates to [packages/simple-vps](packages/simple-vps):

```bash
./install.sh --mode remote --host 203.0.113.10 --ssh-key ~/.ssh/id_ed25519
```

Implementation references:

- [packages/simple-vps/SPEC.md](packages/simple-vps/SPEC.md)
- [packages/cli/SPEC.md](packages/cli/SPEC.md)
