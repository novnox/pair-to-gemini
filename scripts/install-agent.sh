#!/usr/bin/env bash
# Install (or reinstall) the pair-to-gemini proxy as a per-user LaunchAgent.
# The agent must run in the user's GUI session: SSH sessions cannot use the
# Accessibility API, but a LaunchAgent can.
set -euo pipefail

BIN="${PAIR_TO_GEMINI_BIN:-$HOME/.local/bin/pair-to-gemini}"
ROOT="${PAIR_TO_GEMINI_ROOT:-$HOME/.local/share/pair-to-gemini}"
SOCKET="${PAIR_TO_GEMINI_SOCKET:-$ROOT/proxy.sock}"
LABEL="com.pairtogemini.proxy"
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
LOG="$ROOT/logs/proxy.log"

if [ ! -x "$BIN" ]; then
  echo "missing binary: $BIN (run scripts/build.sh first)" >&2
  exit 1
fi

xml_escape() { printf '%s' "$1" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g'; }

mkdir -p "$ROOT/logs" "$(dirname "$PLIST")"
chmod 700 "$ROOT"

cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>$LABEL</string>
	<key>ProgramArguments</key>
	<array>
		<string>$(xml_escape "$BIN")</string>
		<string>serve</string>
		<string>--socket</string>
		<string>$(xml_escape "$SOCKET")</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>WorkingDirectory</key>
	<string>$(xml_escape "$ROOT")</string>
	<key>StandardOutPath</key>
	<string>$(xml_escape "$LOG")</string>
	<key>StandardErrorPath</key>
	<string>$(xml_escape "$LOG")</string>
</dict>
</plist>
EOF
chmod 600 "$PLIST"

launchctl bootout "gui/$UID/$LABEL" 2>/dev/null || true
launchctl bootstrap "gui/$UID" "$PLIST"
launchctl kickstart -k "gui/$UID/$LABEL"
sleep 1

echo "Installed $LABEL (socket: $SOCKET)"
echo "Check: $BIN client -- doctor --json"
echo
echo "If axTrusted is false, open System Settings > Privacy & Security > Accessibility"
echo "and add: $BIN"
