# Real-box smoke runbook

The fake-VPS smoke (`make fake-vps-smoke`, `make fake-vps-install-smoke`)
proves simple-vps's internal shape is consistent against fake Podman
and fake Caddy. This runbook drives the same path against a real
Ubuntu 24.04 VPS with real Podman and real Caddy. Authored from a
live smoke session — every command below was actually run, and every
surprise was filed in [smoke-real-box-results.md](smoke-real-box-results.md).

Run this end-to-end after any change that touches the install path,
the helper-side `app apply` / `app setup-env` verbs, or the Caddy
fragment / Podman networking shape. The fake smoke catches a lot but
not everything — finding 1 (`ufw --force allow`) and finding 3 (UFW
blocking Podman bridge DNS) were both invisible to it.

## 0. Prereqs

- Fresh Ubuntu 24.04 VPS, public IPv4, root SSH from the laptop with
  a known key. Hetzner CX22 (4 GiB RAM, 80 GB disk) is the smallest
  thing that comfortably runs Caddy + a real app container.
- DNS hostname `smoke.<your-domain>` pointing at the VPS IP if you
  want real TLS via Let's Encrypt. **Routing alone works without DNS** —
  curl with a `Host:` header reaches Caddy on port 443 (Caddy auto-
  redirects 80 → 443, so plain HTTP is not the test). Use `tls
  internal` in the fragment for self-signed certs during the smoke;
  see finding 6.
- `simple-vps` built locally:

  ```sh
  make clean && make build
  ```

  `make clean` matters — `prepareGoHelperBinaries` prefers stale
  `dist/` over rebuilding (see PR #30), so always build fresh before
  a smoke run.

- Operator and deploy SSH keys generated for the smoke:

  ```sh
  mkdir -p /tmp/simple-vps-smoke-keys
  ssh-keygen -q -t ed25519 -N '' -f /tmp/simple-vps-smoke-keys/operator
  ssh-keygen -q -t ed25519 -N '' -f /tmp/simple-vps-smoke-keys/deploy
  ```

## 1. Host install

```sh
./dist/simple-vps host install \
  --mode remote \
  --host <IP> \
  --bootstrap-user root \
  --ssh-key ~/.ssh/<root-key> \
  --operator-ssh-public-key-file /tmp/simple-vps-smoke-keys/operator.pub \
  --deploy-ssh-public-key-file /tmp/simple-vps-smoke-keys/deploy.pub \
  --timezone UTC --locale en_US.UTF-8 \
  --no-tailscale --no-cloudflare-tunnel --no-litestream
```

Expected output ends with `==> Provisioning complete` and `Apply
<ID> changed N operations`. If you see `simple-vps: error: ...`
instead, capture the stderr line and add it to results.md.

After install, verify (over SSH as root):

```sh
systemctl is-active caddy           # → active
podman ps                            # → caddy Up, no app yet
podman network ls                    # → ingress, podman
cat /etc/caddy/Caddyfile             # → "import conf.d/*.caddy"
ls /etc/caddy/conf.d/                # → empty
cat /etc/simple-vps/host.json \
  | jq '.meta.last_apply.status'     # → "ok"
```

## 2. Build the test app

```sh
mkdir -p /tmp/simple-vps-smoke-app && cd /tmp/simple-vps-smoke-app

cat > Dockerfile <<'EOF'
FROM docker.io/library/python:3-alpine
RUN mkdir -p /var/www \
 && printf 'smoke-ok' > /var/www/index.html \
 && printf 'ok'       > /var/www/health
WORKDIR /var/www
ENV PYTHONDONTWRITEBYTECODE=1
EXPOSE 3000
CMD ["python3", "-m", "http.server", "3000"]
EOF

cat > simple-vps.toml <<'EOF'
name = "hello"

[env.production]
server = "deploy@<IP>"

[services.web]
port = 3000
healthcheck = "/health"

[routes.app]
host = "smoke.<your-domain>"
type = "proxy"
service = "web"
EOF

git init -q
git config user.email smoke@example.com
git config user.name "Smoke"
git add . && git commit -q -m "fixture"
```

`python:3-alpine` is the smallest image that:

- Has a stateless HTTP server.
- Doesn't try to mkdir into the rootfs (so it survives `--read-only`).
- Doesn't depend on writable `/var/cache` (so it works without extra
  tmpfs mounts).

If you swap it for a different base image, check finding 5 — most
stock distro server images need writable scratch.

## 3. Setup and deploy

```sh
cd /tmp/simple-vps-smoke-app

SIMPLE_VPS_SSH_KEY="$(cat /tmp/simple-vps-smoke-keys/deploy)" \
SIMPLE_VPS_KNOWN_HOSTS="$(ssh-keyscan -t ed25519 -H <IP> 2>/dev/null)" \
  /path/to/simple-vps/dist/simple-vps setup production

# Verify on the box (over SSH as root):
#   getent passwd app-hello-production         # → uid:gid created
#   ls /var/apps/hello/production/             # → shared/
#   podman network ls                           # → app-hello-production

SIMPLE_VPS_SSH_KEY="$(cat /tmp/simple-vps-smoke-keys/deploy)" \
SIMPLE_VPS_KNOWN_HOSTS="$(ssh-keyscan -t ed25519 -H <IP> 2>/dev/null)" \
  /path/to/simple-vps/dist/simple-vps deploy production
```

Expected last line: `Deployed hello (production) at <sha>`. If the
deploy errors with `wget: bad address`, you skipped step 2.

## 4. Bypass Caddy auto-ACME (until DNS is set up)

See finding 6. If you don't have DNS pointing at the box yet, Caddy
will 308-redirect HTTP to HTTPS and then fail the TLS handshake
because Let's Encrypt can't reach the host. Manually inject `tls
internal` into the per-app fragment to use a self-signed cert:

```sh
# As root on the VPS:
fragment=/etc/caddy/conf.d/simple-vps-hello-production.caddy
sed -i 's|reverse_proxy |tls internal\n\treverse_proxy |' "$fragment"
podman exec caddy caddy reload --config /etc/caddy/Caddyfile
```

## 5. Curl through Caddy — the actual test

```sh
curl -k -sS \
  --resolve smoke.<your-domain>:443:<IP> \
  -w "HTTP %{http_code}\n" \
  https://smoke.<your-domain>/health
```

Expected: `HTTP 200` + body `ok`.

```sh
curl -k -sS \
  --resolve smoke.<your-domain>:443:<IP> \
  -w "HTTP %{http_code}\n" \
  https://smoke.<your-domain>/
```

Expected: `HTTP 200` + body `smoke-ok`.

These two responses prove the full path:

```
your curl
  └→ HTTPS to <IP>:443
       └→ Caddy container (on `ingress`, self-signed via `tls internal`)
            └→ aardvark-dns resolves `app-hello-production-web`
                 └→ Podman bridge → app container
                      └→ python3 -m http.server serves /health → 200 ok
```

## 6. Teardown

If the VPS is single-use for this smoke, just delete it from the
provider console. Don't bother running `destroy` — it's not
implemented against the new lifecycle yet.

If you're reusing the box: at minimum, stop and remove the app
container so a future smoke starts clean.

```sh
# As root on the VPS:
podman rm -f app-hello-production-web
podman network rm app-hello-production
userdel app-hello-production
rm -rf /var/apps/hello
rm -f /etc/caddy/conf.d/simple-vps-hello-production.caddy
podman exec caddy caddy reload --config /etc/caddy/Caddyfile
```
