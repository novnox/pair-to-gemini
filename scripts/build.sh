#!/usr/bin/env bash
# Build and install the pair-to-gemini binary.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${PAIR_TO_GEMINI_BIN_DIR:-$HOME/.local/bin}"
ROOT_DIR="${PAIR_TO_GEMINI_ROOT:-$HOME/.local/share/pair-to-gemini}"

mkdir -p "$BIN_DIR" "$ROOT_DIR"
chmod 700 "$ROOT_DIR"

TMP_BIN="$BIN_DIR/.pair-to-gemini.tmp.$$"
(
  cd "$REPO_DIR"
  CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o "$TMP_BIN" ./cmd/pair-to-gemini
)
chmod 755 "$TMP_BIN"

# Sign only when explicitly enabled (marker written by setup-signing.sh, or
# P2G_SIGN_IDENTITY set). Unsigned is the default.
SIGN_IDENTITY="${P2G_SIGN_IDENTITY:-}"
if [ -z "$SIGN_IDENTITY" ] && [ -f "$ROOT_DIR/sign-identity" ]; then
  SIGN_IDENTITY="$(head -n 1 "$ROOT_DIR/sign-identity")"
fi
if [ -n "$SIGN_IDENTITY" ]; then
  if ! security find-certificate -c "$SIGN_IDENTITY" >/dev/null 2>&1; then
    echo "signing identity not found: $SIGN_IDENTITY (run scripts/setup-signing.sh)" >&2
    exit 1
  fi
  codesign --force --sign "$SIGN_IDENTITY" --identifier com.pairtogemini.cli "$TMP_BIN"
  echo "Signed with: $SIGN_IDENTITY"
fi

mv -f "$TMP_BIN" "$BIN_DIR/pair-to-gemini"

echo "Built: $BIN_DIR/pair-to-gemini"
"$BIN_DIR/pair-to-gemini" doctor || true
