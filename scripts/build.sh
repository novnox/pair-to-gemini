#!/usr/bin/env bash
# Build and install pair-to-gemini.
#
#   scripts/build.sh                   build and install the binary
#   scripts/build.sh --enable-signing  create a local signing identity, then build
#   scripts/build.sh --disable-signing stop signing future builds
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${PAIR_TO_GEMINI_BIN_DIR:-$HOME/.local/bin}"
BIN="$BIN_DIR/pair-to-gemini"
ROOT_DIR="${PAIR_TO_GEMINI_ROOT:-$HOME/.local/share/pair-to-gemini}"
SIGN_NAME="${P2G_SIGN_IDENTITY:-pair-to-gemini local}"
KEYCHAIN="${P2G_KEYCHAIN:-$HOME/Library/Keychains/login.keychain-db}"
MARKER="$ROOT_DIR/sign-identity"

# Accessibility (TCC) grants are tied to the binary's code signature. An
# unsigned build changes its cdhash every time, so macOS forgets the grant.
# Signing with one local identity keeps the designated requirement stable.
enable_signing() {
  if ! security find-certificate -c "$SIGN_NAME" "$KEYCHAIN" >/dev/null 2>&1; then
    local work pass
    work="$(mktemp -d)"
    trap 'rm -rf "$work"' RETURN
    pass="$(openssl rand -hex 16)"

    cat >"$work/openssl.cnf" <<EOF
[req]
distinguished_name = dn
x509_extensions = ext
prompt = no
[dn]
CN = $SIGN_NAME
[ext]
basicConstraints = critical,CA:false
keyUsage = critical,digitalSignature
extendedKeyUsage = critical,codeSigning
EOF

    openssl req -x509 -newkey rsa:2048 -sha256 -days 3650 -nodes \
      -keyout "$work/key.pem" -out "$work/cert.pem" -config "$work/openssl.cnf" >/dev/null 2>&1

    # macOS's Security framework cannot read OpenSSL 3's default PBES2 output.
    openssl pkcs12 -export -inkey "$work/key.pem" -in "$work/cert.pem" \
      -name "$SIGN_NAME" -out "$work/id.p12" -passout "pass:$pass" \
      -certpbe PBE-SHA1-3DES -keypbe PBE-SHA1-3DES -macalg sha1 >/dev/null 2>&1

    security import "$work/id.p12" -k "$KEYCHAIN" -P "$pass" -T /usr/bin/codesign >/dev/null
    rm -rf "$work"
    echo "Created code-signing identity: $SIGN_NAME"
  else
    echo "code-signing identity already exists: $SIGN_NAME"
  fi

  mkdir -p "$ROOT_DIR"
  printf '%s\n' "$SIGN_NAME" >"$MARKER"
  chmod 600 "$MARKER"
  echo "Signing enabled for future builds (marker: $MARKER)."
  echo "The first codesign may ask for keychain access; choose Always Allow."
}

case "${1:-}" in
  --enable-signing)
    enable_signing
    ;;
  --disable-signing)
    rm -f "$MARKER"
    echo "Signing disabled for future builds; rebuild to apply."
    echo "The identity stays in the keychain until you delete it in Keychain Access: $SIGN_NAME"
    exit 0
    ;;
  -h | --help)
    sed -n '2,6p' "$0" | sed 's/^# \{0,1\}//'
    exit 0
    ;;
  "")
    ;;
  *)
    echo "unknown option: $1 (try --help)" >&2
    exit 2
    ;;
esac

mkdir -p "$BIN_DIR" "$ROOT_DIR"
chmod 700 "$ROOT_DIR"

TMP_BIN="$BIN_DIR/.pair-to-gemini.tmp.$$"
trap 'rm -f "$TMP_BIN"' EXIT
(
  cd "$REPO_DIR"
  CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o "$TMP_BIN" ./cmd/pair-to-gemini
)
chmod 755 "$TMP_BIN"

SIGN_IDENTITY="${P2G_SIGN_IDENTITY:-}"
if [ -z "$SIGN_IDENTITY" ] && [ -f "$MARKER" ]; then
  SIGN_IDENTITY="$(head -n 1 "$MARKER")"
fi
if [ -n "$SIGN_IDENTITY" ]; then
  if ! security find-certificate -c "$SIGN_IDENTITY" >/dev/null 2>&1; then
    echo "signing identity not found: $SIGN_IDENTITY (run scripts/build.sh --enable-signing)" >&2
    exit 1
  fi
  codesign --force --sign "$SIGN_IDENTITY" --identifier com.pairtogemini.cli "$TMP_BIN"
  echo "Signed with: $SIGN_IDENTITY"
fi

mv -f "$TMP_BIN" "$BIN"

echo "Built: $BIN"
"$BIN" doctor || true
