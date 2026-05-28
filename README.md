# Simple VPS

Simple VPS is one Go CLI for deploying containerized apps to a single hardened
Ubuntu VPS. It is built for solo developers and small teams who want the host
hardening, Caddy routing, Podman runtime, secrets, and deploy workflow handled
without running a dashboard or control plane.

```text
fresh Ubuntu VPS  ->  install.sh         ->  hardened box
your app repo     ->  simple-vps deploy  ->  live app
```

## What Ships Now

- Host install/converge for Ubuntu 24.04.
- Caddy running in a container, with per-app route fragments.
- Podman image builds on the VPS from your app's Dockerfile.
- Per-env Linux users, directories, networks, and mutation locks.
- Manifest env values and host-side `@secret:KEY` resolution.
- `status`, `logs`, `restart`, `destroy`, and JSON read surfaces.
- Fake-VPS smoke tests and a real Ubuntu 24.04 VPS smoke runbook.

Not shipped yet: rollback, backup/restore, `app list --json`, and the planned
`--ingress` / `--admin` preset flags. See [SPEC.md](SPEC.md).

## Start Here

Build the Go CLI locally:

```bash
make build
./dist/simple-vps --help
./dist/simple-vps version
```

Try the local scaffold/check flow without touching this checkout:

```bash
demo=$(mktemp -d /tmp/simple-vps-demo-XXXXXX)
cd "$demo"

/path/to/simple-vps/dist/simple-vps init
/path/to/simple-vps/dist/simple-vps check production
cat simple-vps.toml
```

Run the main checks:

```bash
make test
make fake-vps-smoke
make fake-vps-install-smoke
```

## Install A VPS

Download a release binary, then run `host install`:

```bash
asset=simple-vps-darwin-arm64 # use simple-vps-darwin-amd64 on Intel Macs
curl -fsSL \
  "https://github.com/fprl/simple-vps/releases/download/v0.4.1/${asset}" \
  -o simple-vps
chmod 0755 simple-vps

./simple-vps version
```

From macOS, remote install uploads or downloads the matching Linux helper
binary for the target VPS automatically:

If the release assets are private, set `SIMPLE_VPS_RELEASE_TOKEN`, `GH_TOKEN`,
or `GITHUB_TOKEN` before running remote install.

```bash
./simple-vps host install \
  --mode remote \
  --host 203.0.113.10 \
  --bootstrap-user root \
  --ssh-key ~/.ssh/id_ed25519 \
  --operator-ssh-public-key-file ~/.ssh/id_ed25519.pub \
  --deploy-ssh-public-key-file ~/.ssh/simple-vps-deploy.pub \
  --no-tailscale \
  --no-cloudflare-tunnel \
  --no-litestream \
  --yes
```

The root `install.sh` is a thin bootstrap that finds, downloads, or builds a
local `simple-vps` binary and then runs `simple-vps host install`.

After install, verify the host:

```bash
SIMPLE_VPS_SSH_KEY="$(cat ~/.ssh/simple-vps-deploy)" \
SIMPLE_VPS_KNOWN_HOSTS="$(ssh-keyscan -t ed25519 -H 203.0.113.10 2>/dev/null)" \
  ./simple-vps host status --json --server deploy@203.0.113.10

SIMPLE_VPS_SSH_KEY="$(cat ~/.ssh/simple-vps-deploy)" \
SIMPLE_VPS_KNOWN_HOSTS="$(ssh-keyscan -t ed25519 -H 203.0.113.10 2>/dev/null)" \
  ./simple-vps host doctor --json --server deploy@203.0.113.10
```

## Deploy An App

In an app repo:

```bash
simple-vps init
# edit simple-vps.toml: set [env.production].server and route host
simple-vps check production

simple-vps setup production
simple-vps deploy production
simple-vps status --json production
simple-vps logs production
```

Secrets are stored on the host and referenced from the manifest:

```bash
printf '%s' "$DATABASE_URL" | simple-vps secret put production DATABASE_URL
simple-vps secret list --json production
```

## Release Builds

Build all release binaries:

```bash
make clean
make build-release VERSION=v0.4.1
```

Artifacts land in `dist/`:

```text
simple-vps-linux-amd64
simple-vps-linux-arm64
simple-vps-darwin-amd64
simple-vps-darwin-arm64
```

## References

- [SPEC.md](SPEC.md)
- [CHANGELOG.md](CHANGELOG.md)
- [docs/positioning.md](docs/positioning.md)
- [docs/security-model.md](docs/security-model.md)
- [docs/smoke-real-box.md](docs/smoke-real-box.md)
- [docs/smoke-real-box-results.md](docs/smoke-real-box-results.md)
- [docs/adr/0001-replace-ansible-with-bounded-go-provisioner.md](docs/adr/0001-replace-ansible-with-bounded-go-provisioner.md)
- [docs/adr/0002-state-file-layout.md](docs/adr/0002-state-file-layout.md)
