#!/usr/bin/env bash
# Guided setup. Every question is optional, defaults to "no", and continues
# automatically after 30 seconds.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${PAIR_TO_GEMINI_BIN_DIR:-$HOME/.local/bin}"
BIN="$BIN_DIR/pair-to-gemini"

ask() {
  # $1 prompt; the prompt goes to stderr, the answer to stdout
  local prompt="$1" ans="" deadline
  if [ ! -t 0 ]; then
    printf '%s [y/N] (not interactive: using default N)\n' "$prompt" >&2
    printf ''
    return
  fi
  deadline="$(date -v+30S '+%H:%M:%S' 2>/dev/null || date -d '+30 seconds' '+%H:%M:%S')"
  printf '%s [y/N] (30s timeout, auto-continue at %s, default N): ' "$prompt" "$deadline" >&2
  read -r -t 30 ans || true
  printf '\n' >&2
  printf '%s' "$ans"
}

yes() {
  case "$1" in
    y | Y | yes | YES | Yes) return 0 ;;
    *) return 1 ;;
  esac
}

echo "pair-to-gemini setup"
echo "--------------------"
echo "Questions time out after 30 seconds and continue with the default."
echo

ans="$(ask 'Enable local code signing so Accessibility survives rebuilds?')"
if yes "$ans"; then
  bash "$REPO_DIR/scripts/setup-signing.sh"
else
  echo "Code signing: off (default). Plain builds; re-grant after each rebuild."
fi

bash "$REPO_DIR/scripts/build.sh"

echo
echo "Accessibility is a manual grant:"
echo "  System Settings > Privacy & Security > Accessibility > + > $BIN"
echo "  Then verify: $BIN doctor --json   (axTrusted must be true)"
echo

ans="$(ask 'Install the SSH proxy LaunchAgent now?')"
if yes "$ans"; then
  bash "$REPO_DIR/scripts/install-agent.sh"
else
  echo "SSH proxy: skipped. Run scripts/install-agent.sh when you need it."
fi
