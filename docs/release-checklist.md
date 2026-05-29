# Release Checklist

Use this before cutting `v0.5.0-rc1` and later preview releases.

## Local Checks

```bash
git status --short
make clean
make test
make fake-vps-smoke
make fake-vps-install-smoke
make build-release VERSION=v0.5.0-rc1
(cd dist && shasum -a 256 simple-vps-* > SHA256SUMS)
```

## Example Manifest Checks

```bash
(cd examples/hono-bun-api && ../../dist/simple-vps check production)
(cd examples/astro-static && ../../dist/simple-vps check production)
(cd examples/mixed-api-docs && ../../dist/simple-vps check production)
```

## Real VPS Smoke

Run against a freshly rebuilt Ubuntu 24.04 VPS.

1. Install from the release artifact, not the source checkout:

   ```bash
   tmp=$(mktemp -d /tmp/simple-vps-release-smoke-XXXXXX)
   cd "$tmp"
   curl -fsSL https://raw.githubusercontent.com/fprl/simple-vps/main/install.sh -o install.sh
   chmod 0755 install.sh
   SIMPLE_VPS_VERSION=v0.5.0-rc1 ./install.sh \
     --mode remote \
     --host <ip> \
     --bootstrap-user root \
     --ssh-key ~/.ssh/hetzner \
     --operator-ssh-public-key-file ~/.ssh/hetzner.pub \
     --deploy-ssh-public-key-file ~/.ssh/simple-vps-deploy.pub \
     --ingress public \
     --admin public-ssh \
     --yes
   ```

2. Verify host health:

   ```bash
   SIMPLE_VPS_SSH_KEY="$(cat ~/.ssh/simple-vps-deploy)" \
   SIMPLE_VPS_KNOWN_HOSTS="$(ssh-keyscan -t ed25519 -H <ip> 2>/dev/null)" \
     ./simple-vps host status --json --server deploy@<ip>

   SIMPLE_VPS_SSH_KEY="$(cat ~/.ssh/simple-vps-deploy)" \
   SIMPLE_VPS_KNOWN_HOSTS="$(ssh-keyscan -t ed25519 -H <ip> 2>/dev/null)" \
     ./simple-vps host doctor --json --server deploy@<ip>
   ```

3. Deploy and verify one app of each shape:

   - `examples/hono-bun-api`
   - `examples/astro-static`
   - `examples/mixed-api-docs`

4. For the mixed app, verify:

   ```bash
   simple-vps deploy production
   curl -k --resolve <host>:443:<ip> https://<host>/health
   curl -k --resolve <host>:443:<ip> https://<host>/docs
   simple-vps rollback production
   simple-vps backup production
   simple-vps restore --from <backup-id> production
   simple-vps destroy production --confirm <app> --purge
   simple-vps app list --json --server deploy@<ip>
   ```

## Publish

```bash
git tag -a v0.5.0-rc1 -m "v0.5.0-rc1"
git push origin v0.5.0-rc1
gh release create v0.5.0-rc1 \
  --title "v0.5.0-rc1" \
  --notes-file CHANGELOG.md \
  dist/simple-vps-linux-amd64 \
  dist/simple-vps-linux-arm64 \
  dist/simple-vps-darwin-amd64 \
  dist/simple-vps-darwin-arm64 \
  dist/SHA256SUMS \
  --prerelease
```

After publishing, run the real VPS smoke again from a temp directory with
`SIMPLE_VPS_VERSION=v0.5.0-rc1`.
