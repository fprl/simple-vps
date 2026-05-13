#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

ARCHIVE_PATH="$TMP_DIR/simple-vps.tar.gz"
BARE_DIR="$TMP_DIR/bare"

git -C "$ROOT_DIR" archive --format=tar.gz --prefix=simple-vps-main/ --output "$ARCHIVE_PATH" HEAD
mkdir -p "$BARE_DIR"
cp "$ROOT_DIR/install.sh" "$BARE_DIR/install.sh"

set +e
OUTPUT="$(
  cd "$BARE_DIR" && \
    SIMPLE_VPS_REPO_TARBALL_URL="file://$ARCHIVE_PATH" \
      ./install.sh \
        --mode remote \
        --host 127.0.0.1 \
        --yes \
        --no-tailscale \
        --no-cloudflare-tunnel 2>&1
)"
STATUS=$?
set -e

if [[ "$STATUS" -eq 0 ]]; then
  printf 'Expected bootstrap smoke test to fail at SSH preflight.\n' >&2
  exit 1
fi

if ! grep -Fq "Re-running installer from downloaded checkout." <<<"$OUTPUT"; then
  printf 'Installer did not re-exec from downloaded checkout.\n' >&2
  printf '%s\n' "$OUTPUT" >&2
  exit 1
fi

if ! grep -Fq "SSH preflight failed for root@127.0.0.1." <<<"$OUTPUT"; then
  printf 'Installer did not reach expected SSH preflight failure.\n' >&2
  printf '%s\n' "$OUTPUT" >&2
  exit 1
fi

printf 'Bootstrap tarball smoke test passed.\n'
