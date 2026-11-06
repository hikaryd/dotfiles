#!/usr/bin/env bash
set -euo pipefail
umask 077
if [[ $(uname -s) != Darwin || ( ! -d /Applications/Rex\ Beta.app && ! -d /Applications/Rex.app ) ]]; then
  echo 'Rex last-session: macOS Rex app not found, skipping.'
  exit 0
fi
command -v swiftc >/dev/null || { echo 'Swift compiler not found' >&2; exit 1; }
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
bin="$HOME/.local/bin"
agents="$HOME/Library/LaunchAgents"
cache="$HOME/Library/Caches/dots-rex-last-session"
mkdir -p "$bin" "$agents" "$cache"
chmod 700 "$cache"
tmp=$(mktemp "$bin/.dots-rex-last-session.XXXXXX")
plist=$(mktemp "$agents/.local.dots.rex-last-session.XXXXXX")
trap 'rm -f "$tmp" "$plist"' EXIT
swiftc "$root/tools/rex-last-session/main.swift" -o "$tmp"
/usr/bin/plutil -create xml1 "$plist"
/usr/bin/plutil -insert Label -string local.dots.rex-last-session "$plist"
/usr/bin/plutil -insert ProgramArguments -json '[]' "$plist"
/usr/bin/plutil -insert ProgramArguments.0 -string "$bin/dots-rex-last-session" "$plist"
/usr/bin/plutil -insert ProgramArguments.1 -string watch "$plist"
/usr/bin/plutil -insert RunAtLoad -bool YES "$plist"
/usr/bin/plutil -insert KeepAlive -bool YES "$plist"
/usr/bin/plutil -insert ThrottleInterval -integer 15 "$plist"
/usr/bin/plutil -insert StandardErrorPath -string "$cache/watcher.log" "$plist"
touch "$cache/watcher.log"
chmod 600 "$cache/watcher.log"
domain="gui/$(id -u)"
if launchctl print "$domain/local.dots.rex-last-session" >/dev/null 2>&1; then
  launchctl bootout "$domain/local.dots.rex-last-session"
fi
mv -f "$tmp" "$bin/dots-rex-last-session"
mv -f "$plist" "$agents/local.dots.rex-last-session.plist"
launchctl bootstrap "$domain" "$agents/local.dots.rex-last-session.plist"
printf 'Run %s request-access and grant Accessibility permission in System Settings.\n' "$bin/dots-rex-last-session"
