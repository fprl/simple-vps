#!/usr/bin/env bash
set -euo pipefail

SIMPLE_VPS_VERSION="${SIMPLE_VPS_VERSION:-v0.7.0}"
SIMPLE_VPS_RELEASE_BASE_URL="${SIMPLE_VPS_RELEASE_BASE_URL:-https://github.com/fprl/simple-vps/releases/download}"
SIMPLE_VPS_RELEASE_API_BASE_URL="${SIMPLE_VPS_RELEASE_API_BASE_URL:-https://api.github.com/repos/fprl/simple-vps}"
SIMPLE_VPS_INSTALL_DIR="${SIMPLE_VPS_INSTALL_DIR:-$HOME/.local/bin}"

tmp_dir=""

usage() {
  cat <<'USAGE'
Usage:
  curl -fsSL https://github.com/fprl/simple-vps/releases/download/v0.7.0/install.sh | bash

  install.sh [--version v0.7.0] [--bin-dir ~/.local/bin]

Installs the simple-vps CLI on this machine. It does not provision a VPS.
After this, run:

  test -f ~/.ssh/simple-vps-deploy || ssh-keygen -q -t ed25519 -N '' -f ~/.ssh/simple-vps-deploy
  test -f ~/.ssh/simple-vps-deploy.pub || ssh-keygen -y -f ~/.ssh/simple-vps-deploy > ~/.ssh/simple-vps-deploy.pub
  simple-vps host install --host <vps-ip> --ssh-key ~/.ssh/<root-key>

Environment:
  SIMPLE_VPS_VERSION
      release tag to install, default v0.7.0
  SIMPLE_VPS_INSTALL_DIR
      install directory, default ~/.local/bin
  SIMPLE_VPS_RELEASE_TOKEN, GH_TOKEN, or GITHUB_TOKEN
      optional token for private release assets
USAGE
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

info() {
  printf '==> %s\n' "$*" >&2
}

cleanup() {
  if [[ -n "$tmp_dir" ]]; then
    rm -rf "$tmp_dir"
  fi
}
trap cleanup EXIT

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      [[ $# -ge 2 ]] || die "--version requires a value"
      SIMPLE_VPS_VERSION="$2"
      shift 2
      ;;
    --bin-dir)
      [[ $# -ge 2 ]] || die "--bin-dir requires a value"
      SIMPLE_VPS_INSTALL_DIR="$2"
      shift 2
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

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required"
}

platform_asset() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "$os" in
    darwin|linux) ;;
    *) die "unsupported OS: $os" ;;
  esac

  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) die "unsupported architecture: $arch" ;;
  esac

  printf 'simple-vps-%s-%s\n' "$os" "$arch"
}

token() {
  printf '%s' "${SIMPLE_VPS_RELEASE_TOKEN:-${GH_TOKEN:-${GITHUB_TOKEN:-}}}"
}

curl_download_quiet() {
  local url="$1"
  local output="$2"
  local auth_token
  auth_token="$(token)"
  if [[ -n "$auth_token" ]]; then
    curl -fsSL -H "Authorization: Bearer $auth_token" "$url" -o "$output" 2>/dev/null
  else
    curl -fsSL "$url" -o "$output" 2>/dev/null
  fi
}

download_via_github_api() {
  local asset_name="$1"
  local output="$2"
  local auth_token
  local release_json
  local asset_url

  auth_token="$(token)"
  [[ -n "$auth_token" ]] || return 1
  require_cmd python3

  release_json="$tmp_dir/release.json"
  curl -fsSL \
    -H "Authorization: Bearer $auth_token" \
    -H "Accept: application/vnd.github+json" \
    "${SIMPLE_VPS_RELEASE_API_BASE_URL%/}/releases/tags/$SIMPLE_VPS_VERSION" \
    -o "$release_json"

  asset_url="$(python3 - "$asset_name" "$release_json" <<'PY'
import json
import sys

name = sys.argv[1]
path = sys.argv[2]
with open(path, "r", encoding="utf-8") as f:
    release = json.load(f)
for asset in release.get("assets", []):
    if asset.get("name") == name:
        print(asset["url"])
        break
PY
)"
  [[ -n "$asset_url" ]] || die "release $SIMPLE_VPS_VERSION does not contain $asset_name"

  curl -fsSL \
    -H "Authorization: Bearer $auth_token" \
    -H "Accept: application/octet-stream" \
    "$asset_url" \
    -o "$output"
}

download_release_asset() {
  local asset_name="$1"
  local output="$2"
  local url
  url="${SIMPLE_VPS_RELEASE_BASE_URL%/}/$SIMPLE_VPS_VERSION/$asset_name"

  if curl_download_quiet "$url" "$output"; then
    return 0
  fi
  download_via_github_api "$asset_name" "$output" || die "download failed: $url"
}

verify_checksum() {
  local binary="$1"
  local asset_name="$2"
  local sums="$3"
  local expected actual

  expected="$(awk -v asset="$asset_name" '$2 == asset || $2 == "*" asset { print $1; exit }' "$sums")"
  [[ -n "$expected" ]] || die "SHA256SUMS does not contain $asset_name"

  if command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$binary" | awk '{ print $1 }')"
  elif command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$binary" | awk '{ print $1 }')"
  else
    die "shasum or sha256sum is required"
  fi

  [[ "$actual" == "$expected" ]] || die "checksum mismatch for $asset_name"
}

main() {
  local asset binary sums target resolved
  require_cmd curl
  require_cmd awk
  require_cmd install

  asset="$(platform_asset)"
  tmp_dir="$(mktemp -d)"
  binary="$tmp_dir/simple-vps"
  sums="$tmp_dir/SHA256SUMS"
  target="$SIMPLE_VPS_INSTALL_DIR/simple-vps"

  info "Installing simple-vps $SIMPLE_VPS_VERSION for $asset"
  download_release_asset "$asset" "$binary"
  download_release_asset "SHA256SUMS" "$sums"
  verify_checksum "$binary" "$asset" "$sums"

  mkdir -p "$SIMPLE_VPS_INSTALL_DIR"
  install -m 0755 "$binary" "$target"

  info "Installed $target"
  resolved="$(command -v simple-vps 2>/dev/null || true)"
  if [[ "$resolved" != "$target" ]]; then
    if [[ -n "$resolved" ]]; then
      printf '%s\n' "Your shell currently resolves simple-vps to:" >&2
      printf '%s\n' "  $resolved" >&2
    fi
    printf '%s\n' "Add this to your shell profile so this install wins first:" >&2
    printf '%s\n' "  export PATH=\"$SIMPLE_VPS_INSTALL_DIR:\$PATH\"" >&2
    printf '%s\n' "Run this now for the current shell:" >&2
    printf '%s\n' "  export PATH=\"$SIMPLE_VPS_INSTALL_DIR:\$PATH\"" >&2
  fi
  "$target" version
}

main "$@"
