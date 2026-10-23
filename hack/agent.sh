#!/bin/bash
# Run kubesilicond at login as a LaunchAgent.
# Usage: hack/agent.sh install <kube-context> | uninstall
# The binary is copied to ~/.kubesilicon/bin so launchd does not need access to this checkout
# (macOS protects ~/Documents and similar folders).
set -euo pipefail
cd "$(dirname "$0")/.."
LABEL=io.github.matheus-thurler.kubesilicond
PLIST=$HOME/Library/LaunchAgents/$LABEL.plist
DEST=$HOME/.kubesilicon/bin
DOMAIN=gui/$(id -u)

case "${1:-}" in
install)
  ctx=${2:?usage: hack/agent.sh install <kube-context>}
  make -s bin/kubesilicond
  mkdir -p "$DEST" "$(dirname "$PLIST")"
  cp bin/kubesilicond "$DEST/kubesilicond"
  codesign --force --sign - --entitlements hack/vz.entitlements "$DEST/kubesilicond" 2>/dev/null ||
    codesign --force --sign - --entitlements hack/vz.entitlements "$DEST/kubesilicond"
  # AbandonProcessGroup: stopping the agent must not take the VM launchers down with it.
  cat >"$PLIST" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>$LABEL</string>
  <key>ProgramArguments</key><array>
    <string>$DEST/kubesilicond</string><string>-context</string><string>$ctx</string>
  </array>
  <key>EnvironmentVariables</key><dict>
    <key>PATH</key><string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>AbandonProcessGroup</key><true/>
  <key>StandardOutPath</key><string>$HOME/.kubesilicon/kubesilicond.log</string>
  <key>StandardErrorPath</key><string>$HOME/.kubesilicon/kubesilicond.log</string>
</dict></plist>
PLIST
  launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
  while launchctl print "$DOMAIN/$LABEL" >/dev/null 2>&1; do sleep 0.5; done # bootout is async
  launchctl bootstrap "$DOMAIN" "$PLIST"
  echo "daemon running in the background (context $ctx). Log: ~/.kubesilicon/kubesilicond.log"
  ;;
uninstall)
  launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
  while launchctl print "$DOMAIN/$LABEL" >/dev/null 2>&1; do sleep 0.5; done
  rm -f "$PLIST"
  echo "daemon removed (running VMs keep running)"
  ;;
*) echo "usage: hack/agent.sh install <kube-context> | uninstall" >&2; exit 2 ;;
esac
