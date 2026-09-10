#!/bin/sh
set -e

LABEL=io.zznq.grabkernel2xpf
ADDR="${ADDR:-127.0.0.1:8787}"
ROOT=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
DOMAIN="gui/$(id -u)"
LOG="$ROOT/server.log"

bootout() {
	launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
	launchctl unload "$PLIST" 2>/dev/null || true
}

uninstall() {
	bootout
	rm -f "$PLIST"
	echo "uninstalled $LABEL"
}

restart() {
	if [ ! -x "$ROOT/server" ]; then
		echo "missing $ROOT/server (run make server first)" >&2
		exit 1
	fi
	if [ ! -f "$PLIST" ]; then
		install
		return
	fi
	if launchctl kickstart -k "$DOMAIN/$LABEL" 2>/dev/null; then
		echo "restarted $LABEL"
		echo "listening on http://$ADDR"
		echo "logs $LOG"
		return
	fi
	install
}

install() {
	if [ ! -x "$ROOT/server" ]; then
		echo "missing $ROOT/server (run make server first)" >&2
		exit 1
	fi
	mkdir -p "$HOME/Library/LaunchAgents" "$ROOT/data"
	bootout
	cat >"$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>$LABEL</string>
	<key>WorkingDirectory</key>
	<string>$ROOT</string>
	<key>ProgramArguments</key>
	<array>
		<string>$ROOT/server</string>
		<string>-addr</string>
		<string>$ADDR</string>
		<string>-data</string>
		<string>$ROOT/data</string>
		<string>-external</string>
		<string>$ROOT/external/_bin</string>
		<string>-web</string>
		<string>$ROOT/web</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>$LOG</string>
	<key>StandardErrorPath</key>
	<string>$LOG</string>
</dict>
</plist>
EOF
	if ! launchctl bootstrap "$DOMAIN" "$PLIST" 2>/dev/null; then
		launchctl load -w "$PLIST"
	fi
	echo "installed $LABEL"
	echo "listening on http://$ADDR"
	echo "logs $LOG"
}

case "${1:-install}" in
install) install ;;
restart|reload|update) restart ;;
uninstall|stop) uninstall ;;
*)
	echo "usage: $0 [install|restart|uninstall]" >&2
	exit 2
	;;
esac
