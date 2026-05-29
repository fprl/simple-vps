# Release Checklist

Use this before cutting preview releases.

```bash
VERSION=v0.5.0-rc3
```

## Local Checks

```bash
git status --short
make clean
make test
make fake-vps-smoke
make fake-vps-install-smoke
make build-release VERSION="$VERSION"
make build VERSION="$VERSION"
```

## Example Manifest Checks

```bash
(cd examples/hono-bun-api && ../../dist/simple-vps check --env production)
(cd examples/php-plain && ../../dist/simple-vps check --env production)
(cd examples/astro-static && ../../dist/simple-vps check --env production)
(cd examples/mixed-api-docs && ../../dist/simple-vps check --env production)
tmp=$(mktemp -d /tmp/simple-vps-init-check-XXXXXX)
./dist/simple-vps init --config "$tmp/simple-vps.toml" --template php --name init-php --server deploy@example.com --host init-php.example.com
./dist/simple-vps check --config "$tmp/simple-vps.toml" --env production

# Optional local container build coverage when Podman is available:
SIMPLE_VPS_TEST_INIT_BUILDS=1 go test ./cmd/client -run TestRunInitGeneratedContainerTemplatesBuildWhenRequested
```

## Real VPS Smoke

Run against a freshly rebuilt Ubuntu 24.04 or 26.04 VPS.
Requires `curl`, `git`, `jq`, and `ssh-keyscan` on the smoke machine.

1. Install from the release artifact, not the source checkout:

   ```bash
   tmp=$(mktemp -d /tmp/simple-vps-release-smoke-XXXXXX)
   cd "$tmp"
   curl -fsSL "https://raw.githubusercontent.com/fprl/simple-vps/$VERSION/install.sh" -o install.sh
   chmod 0755 install.sh
   SIMPLE_VPS_VERSION="$VERSION" ./install.sh \
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

   For private release assets, export `SIMPLE_VPS_RELEASE_TOKEN`, `GH_TOKEN`, or
   `GITHUB_TOKEN`. If the repository itself is private, fetch the installer
   through the GitHub Contents API instead of `raw.githubusercontent.com`:

   ```bash
   curl -fsSL \
     -H "Authorization: Bearer $SIMPLE_VPS_RELEASE_TOKEN" \
     -H "Accept: application/vnd.github.raw" \
     "https://api.github.com/repos/fprl/simple-vps/contents/install.sh?ref=$VERSION" \
     -o install.sh
   ```

2. Download the release client binary into the smoke directory. The installer
   uses a temp client and cleans it up, so do not expect `./simple-vps` to exist
   after `install.sh` exits.

   ```bash
   token="${SIMPLE_VPS_RELEASE_TOKEN:-${GH_TOKEN:-${GITHUB_TOKEN:-}}}"
   auth_args=()
   if [[ -n "$token" ]]; then
     auth_args=(-H "Authorization: Bearer $token")
   fi

   case "$(uname -s)-$(uname -m)" in
     Darwin-arm64) asset=simple-vps-darwin-arm64 ;;
     Darwin-x86_64) asset=simple-vps-darwin-amd64 ;;
     Linux-arm64|Linux-aarch64) asset=simple-vps-linux-arm64 ;;
     Linux-x86_64) asset=simple-vps-linux-amd64 ;;
     *) echo "unsupported smoke host: $(uname -s)-$(uname -m)" >&2; exit 1 ;;
   esac

   release_json="$(curl -fsSL \
     "${auth_args[@]}" \
     -H "Accept: application/vnd.github+json" \
     "https://api.github.com/repos/fprl/simple-vps/releases/tags/$VERSION")"
   asset_url="$(printf '%s' "$release_json" | jq -r --arg name "$asset" '.assets[] | select(.name == $name) | .url')"
   sums_url="$(printf '%s' "$release_json" | jq -r '.assets[] | select(.name == "SHA256SUMS") | .url')"
   curl -fsSL \
     "${auth_args[@]}" \
     -H "Accept: application/octet-stream" \
     "$asset_url" \
     -o simple-vps
   curl -fsSL \
     "${auth_args[@]}" \
     -H "Accept: application/octet-stream" \
     "$sums_url" \
     -o SHA256SUMS
   chmod 0755 simple-vps
   SIMPLE_VPS="$tmp/simple-vps"

   if command -v sha256sum >/dev/null 2>&1; then
     grep "  $asset$" SHA256SUMS | sed "s/$asset/simple-vps/" | sha256sum -c -
   else
     grep "  $asset$" SHA256SUMS | sed "s/$asset/simple-vps/" | shasum -a 256 -c -
   fi
   "$SIMPLE_VPS" version
   ```

3. Verify host health:

   ```bash
   SIMPLE_VPS_SSH_KEY="$(cat ~/.ssh/simple-vps-deploy)" \
   SIMPLE_VPS_KNOWN_HOSTS="$(ssh-keyscan -t ed25519 -H <ip> 2>/dev/null)" \
     "$SIMPLE_VPS" host status --json --server deploy@<ip>

   SIMPLE_VPS_SSH_KEY="$(cat ~/.ssh/simple-vps-deploy)" \
   SIMPLE_VPS_KNOWN_HOSTS="$(ssh-keyscan -t ed25519 -H <ip> 2>/dev/null)" \
     "$SIMPLE_VPS" host doctor --json --server deploy@<ip>
   ```

4. Deploy and verify one app of each shape:

   - `examples/hono-bun-api`
   - `examples/php-plain`
   - `examples/astro-static`
   - `examples/mixed-api-docs`

   `hono-bun-api` and `php-plain` are both container apps, but smoke both
   before a preview release so the examples cover more than one runtime.

5. For the mixed app, verify:

   ```bash
   "$SIMPLE_VPS" deploy --env production
   curl -k --resolve <host>:443:<ip> https://<host>/health
   curl -k --resolve <host>:443:<ip> https://<host>/docs
   "$SIMPLE_VPS" rollback --env production
   "$SIMPLE_VPS" backup create --env production
   "$SIMPLE_VPS" restore --from <backup-id> --env production
   "$SIMPLE_VPS" destroy --env production --confirm <app> --purge
   "$SIMPLE_VPS" app list --json --server deploy@<ip>
   ```

## Publish

```bash
git tag -a "$VERSION" -m "$VERSION"
git push origin "$VERSION"
```

The `Release` GitHub Actions workflow builds the release assets, generates
`SHA256SUMS`, creates or updates the GitHub release, and uploads the assets with
`--clobber`.

After publishing, run the real VPS smoke again from a temp directory with
`SIMPLE_VPS_VERSION="$VERSION"`.
