#!/usr/bin/env bash
# Create a local self-signed code-signing identity (run once).
#
# Accessibility (TCC) grants are tied to the binary's code signature. An
# unsigned or ad-hoc binary changes its cdhash on every rebuild, so macOS
# forgets the grant. Signing every build with the same local certificate keeps
# the designated requirement stable, so the grant survives rebuilds.
set -euo pipefail

NAME="${P2G_SIGN_IDENTITY:-pair-to-gemini local}"
KEYCHAIN="${P2G_KEYCHAIN:-$HOME/Library/Keychains/login.keychain-db}"
ROOT="${PAIR_TO_GEMINI_ROOT:-$HOME/.local/share/pair-to-gemini}"
MARKER="$ROOT/sign-identity"

if [ "${1:-}" = "--disable" ]; then
  rm -f "$MARKER"
  echo "Signing disabled for future builds; rebuild to apply."
  echo "The existing identity stays in the keychain until you delete it in Keychain Access: $NAME"
  exit 0
fi

if ! security find-certificate -c "$NAME" "$KEYCHAIN" >/dev/null 2>&1; then
  WORK="$(mktemp -d)"
  trap 'rm -rf "$WORK"' EXIT
  PASS="$(openssl rand -hex 16)"

  cat > "$WORK/openssl.cnf" <<EOF
[req]
distinguished_name = dn
x509_extensions = ext
prompt = no
[dn]
CN = $NAME
[ext]
basicConstraints = critical,CA:false
keyUsage = critical,digitalSignature
extendedKeyUsage = critical,codeSigning
EOF

  openssl req -x509 -newkey rsa:2048 -sha256 -days 3650 -nodes \
    -keyout "$WORK/key.pem" -out "$WORK/cert.pem" -config "$WORK/openssl.cnf" >/dev/null 2>&1

  # macOS's Security framework cannot read OpenSSL 3's default PBES2 output.
  openssl pkcs12 -export -inkey "$WORK/key.pem" -in "$WORK/cert.pem" \
    -name "$NAME" -out "$WORK/id.p12" -passout "pass:$PASS" \
    -certpbe PBE-SHA1-3DES -keypbe PBE-SHA1-3DES -macalg sha1 >/dev/null 2>&1

  security import "$WORK/id.p12" -k "$KEYCHAIN" -P "$PASS" -T /usr/bin/codesign >/dev/null
  echo "Created code-signing identity: $NAME"
else
  echo "code-signing identity already exists: $NAME"
fi

mkdir -p "$ROOT"
printf '%s\n' "$NAME" > "$MARKER"
chmod 600 "$MARKER"

echo "Signing enabled for future builds (marker: $MARKER)."
echo
echo "Next:"
echo "  1. scripts/build.sh        (signs the binary)"
echo "  2. Re-add ~/.local/bin/pair-to-gemini to Accessibility once."
echo "     Later rebuilds keep the grant because the identity is stable."
echo "  3. The first codesign may ask for keychain access; choose Always Allow."
echo
echo "Opt out later: scripts/setup-signing.sh --disable"
echo "Remove the certificate in Keychain Access (login keychain): $NAME"
