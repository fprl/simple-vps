#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/release-smoke.sh --version v0.7.0 --host 128.140.3.159

Runs a release-artifact smoke against one VPS:
  - fetch installer from the release
  - install the local CLI through that installer
  - run remote host install
  - run host status/doctor
  - generate a PHP app with simple-vps init
  - setup, deploy, curl, destroy

Environment:
  SIMPLE_VPS_RELEASE_TOKEN, GH_TOKEN, or GITHUB_TOKEN
      optional for public releases, required for private release assets
  SIMPLE_VPS_BOOTSTRAP_USER
      defaults to root
  SIMPLE_VPS_BOOTSTRAP_SSH_KEY
      defaults to ~/.ssh/hetzner
  SIMPLE_VPS_OPERATOR_PUBKEY
      defaults to ~/.ssh/hetzner.pub
  SIMPLE_VPS_DEPLOY_PUBKEY
      defaults to ~/.ssh/simple-vps-deploy.pub
  SIMPLE_VPS_DEPLOY_SSH_KEY
      defaults to ~/.ssh/simple-vps-deploy
  SIMPLE_VPS_SMOKE_APP
      defaults to svps-smoke-<utc time>
  SIMPLE_VPS_SMOKE_ROUTE_HOST
      defaults to <app>.<host>.nip.io
  SIMPLE_VPS_SMOKE_SKIP_INSTALL=1
      skip host install and only run the app smoke
  SIMPLE_VPS_SMOKE_REFRESH_KNOWN_HOSTS=0
      do not refresh ~/.ssh/known_hosts for the disposable VPS
USAGE
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required"
}

api_get() {
  curl -fsSL "${auth_args[@]}" "$@"
}

download_installer() {
  local asset_url

  if [[ ${#auth_args[@]} -gt 0 ]]; then
    release_json="$(api_get \
      -H "Accept: application/vnd.github+json" \
      "https://api.github.com/repos/fprl/simple-vps/releases/tags/$version")"
    asset_url="$(printf '%s' "$release_json" | jq -r '.assets[] | select(.name == "install.sh") | .url')"
    [[ -n "$asset_url" && "$asset_url" != "null" ]] || die "release asset not found: install.sh"
    api_get \
      -H "Accept: application/octet-stream" \
      "$asset_url" \
      -o install.sh
  else
    curl -fsSL "https://github.com/fprl/simple-vps/releases/download/$version/install.sh" -o install.sh
  fi
  chmod 0755 install.sh
}

version="${VERSION:-}"
host="${SIMPLE_VPS_SMOKE_HOST:-}"
skip_install="${SIMPLE_VPS_SMOKE_SKIP_INSTALL:-0}"
refresh_known_hosts="${SIMPLE_VPS_SMOKE_REFRESH_KNOWN_HOSTS:-1}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      [[ $# -ge 2 ]] || die "--version requires a value"
      version="$2"
      shift 2
      ;;
    --host)
      [[ $# -ge 2 ]] || die "--host requires a value"
      host="$2"
      shift 2
      ;;
    --skip-install)
      skip_install=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

[[ -n "$version" ]] || die "--version or VERSION is required"
[[ -n "$host" ]] || die "--host or SIMPLE_VPS_SMOKE_HOST is required"

require_cmd curl
require_cmd git
require_cmd jq
require_cmd ssh-keygen
require_cmd ssh-keyscan

token="${SIMPLE_VPS_RELEASE_TOKEN:-${GH_TOKEN:-${GITHUB_TOKEN:-}}}"
auth_args=()
if [[ -n "$token" ]]; then
  auth_args=(-H "Authorization: Bearer $token")
fi

bootstrap_user="${SIMPLE_VPS_BOOTSTRAP_USER:-root}"
bootstrap_key="${SIMPLE_VPS_BOOTSTRAP_SSH_KEY:-$HOME/.ssh/hetzner}"
operator_pubkey="${SIMPLE_VPS_OPERATOR_PUBKEY:-$HOME/.ssh/hetzner.pub}"
deploy_pubkey="${SIMPLE_VPS_DEPLOY_PUBKEY:-$HOME/.ssh/simple-vps-deploy.pub}"
deploy_key="${SIMPLE_VPS_DEPLOY_SSH_KEY:-$HOME/.ssh/simple-vps-deploy}"

if [[ "$skip_install" != "1" ]]; then
  [[ -r "$bootstrap_key" ]] || die "bootstrap SSH key not readable: $bootstrap_key"
  [[ -r "$operator_pubkey" ]] || die "operator public key not readable: $operator_pubkey"
  [[ -r "$deploy_pubkey" ]] || die "deploy public key not readable: $deploy_pubkey"
fi
[[ -r "$deploy_key" ]] || die "deploy SSH key not readable: $deploy_key"

app="${SIMPLE_VPS_SMOKE_APP:-svps-smoke-$(date -u +%H%M%S)}"
route_host="${SIMPLE_VPS_SMOKE_ROUTE_HOST:-$app.$host.nip.io}"
server="deploy@$host"
workdir="${SIMPLE_VPS_SMOKE_WORKDIR:-$(mktemp -d /tmp/simple-vps-release-smoke-XXXXXX)}"
client="$workdir/bin/simple-vps"
app_dir="$workdir/app"
log="$workdir/release-smoke.log"

cleanup() {
  if [[ -x "$client" && -f "$app_dir/simple-vps.toml" ]]; then
    "$client" destroy --config "$app_dir/simple-vps.toml" --env production --confirm "$app" --purge >>"$log" 2>&1 || true
  fi
}
trap cleanup EXIT

mkdir -p "$workdir"
cd "$workdir"

run_smoke() {
  printf 'release smoke workdir: %s\n' "$workdir"
  printf 'release: %s\n' "$version"
  printf 'host: %s\n' "$host"
  printf 'app: %s\n' "$app"
  printf 'route host: %s\n' "$route_host"

  download_installer
  mkdir -p "$workdir/bin"
  SIMPLE_VPS_VERSION="$version" \
    SIMPLE_VPS_INSTALL_DIR="$workdir/bin" \
    SIMPLE_VPS_RELEASE_TOKEN="$token" \
    ./install.sh
  "$client" version

  if [[ "$refresh_known_hosts" == "1" ]]; then
    ssh-keygen -R "$host" >/dev/null 2>&1 || true
    ssh-keyscan -T 10 -t ed25519,rsa,ecdsa "$host" >>"$HOME/.ssh/known_hosts"
  fi

  if [[ "$skip_install" != "1" ]]; then
    "$client" host install \
      --host "$host" \
      --bootstrap-user "$bootstrap_user" \
      --ssh-key "$bootstrap_key" \
      --operator-ssh-public-key-file "$operator_pubkey" \
      --deploy-ssh-public-key-file "$deploy_pubkey" \
      --ingress public \
      --admin public-ssh \
      --yes
  else
    printf 'skipping host install\n'
  fi

  if [[ "$deploy_key" != "$HOME/.ssh/simple-vps-deploy" ]]; then
    SIMPLE_VPS_SSH_KEY="$(cat "$deploy_key")"
    export SIMPLE_VPS_SSH_KEY
  fi

  "$client" host status --json --server "$server" >/dev/null
  "$client" host doctor --json --server "$server" >/dev/null

  rm -rf "$app_dir"
  mkdir -p "$app_dir"
  cd "$app_dir"
  "$client" init \
    --template php \
    --name "$app" \
    --server "$server" \
    --host "$route_host" \
    --tls internal
  git init >/dev/null
  git config user.email smoke@example.com
  git config user.name Smoke
  git add .
  git commit -m "release smoke" >/dev/null

  "$client" check --env production
  "$client" setup --env production
  "$client" deploy --env production

  curl -ksS --resolve "$route_host:443:$host" "https://$route_host/health" -o "$workdir/health.out"
  curl -ksS --resolve "$route_host:443:$host" "https://$route_host/" -o "$workdir/body.out"
  grep -q '^ok$' "$workdir/health.out"
  grep -q '"app":"'"$app"'"' "$workdir/body.out"
  printf '/health -> %s\n' "$(cat "$workdir/health.out")"
  printf '/       -> %s\n' "$(cat "$workdir/body.out")"

  "$client" destroy --env production --confirm "$app" --purge
  cd "$workdir"
  "$client" app list --server "$server" --json >"$workdir/app-list.json"
  printf 'app list --json -> %s\n' "$(tr -d '\n' <"$workdir/app-list.json")"
  jq -e --arg app "$app" --arg env production 'all(.apps[]?; .app != $app or .env != $env)' "$workdir/app-list.json" >/dev/null
}

run_smoke > >(tee "$log") 2>&1

trap - EXIT
printf 'release smoke passed\n'
printf 'workdir: %s\n' "$workdir"
printf 'log: %s\n' "$log"
