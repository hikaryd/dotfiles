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
source_file="$root/tools/rex-last-session/main.swift"
helper="$bin/dots-rex-last-session"
marker="$helper.sha256"
installed_plist="$agents/local.dots.rex-last-session.plist"
fingerprint=$(shasum -a 256 "$source_file" | cut -d ' ' -f 1)
rebuild=0
if [[ ! -x $helper || ! -f $marker || $(< "$marker") != "$fingerprint" ]]; then rebuild=1; fi
tmp=$(mktemp "$bin/.dots-rex-last-session.XXXXXX")
plist=$(mktemp "$agents/.local.dots.rex-last-session.XXXXXX")
new_marker=$(mktemp "$bin/.dots-rex-last-session.sha256.XXXXXX")
trap 'rm -f "$tmp" "$plist" "$new_marker"' EXIT
if (( rebuild )); then
  swiftc "$source_file" -o "$tmp"
  printf '%s\n' "$fingerprint" > "$new_marker"
fi
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
loaded=0
running=0
if status=$(launchctl print "$domain/local.dots.rex-last-session" 2>/dev/null); then
  loaded=1
  if grep -Eq '^[[:space:]]*state = running[[:space:]]*$' <<< "$status" &&
     grep -Eq '^[[:space:]]*pid = [1-9][0-9]*[[:space:]]*$' <<< "$status"; then running=1; fi
fi
plist_changed=0
if ! cmp -s "$plist" "$installed_plist"; then plist_changed=1; fi
if (( loaded && (rebuild || plist_changed) )); then
  launchctl bootout "$domain/local.dots.rex-last-session"
  loaded=0
  running=0
fi
if (( rebuild )); then
  rm -f "$marker"
  mv -f "$tmp" "$helper"
fi
if (( plist_changed )); then mv -f "$plist" "$installed_plist"; fi
if (( ! loaded )); then
  launchctl bootstrap "$domain" "$installed_plist"
elif (( ! running )); then
  launchctl kickstart "$domain/local.dots.rex-last-session"
fi
if (( rebuild )); then mv -f "$new_marker" "$marker"; fi
printf 'Run %s request-access and grant Accessibility permission in System Settings.\n' "$bin/dots-rex-last-session"
