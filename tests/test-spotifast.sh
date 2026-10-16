#!/usr/bin/env bash
# Isolated contracts: no real apps, proxy processes, or user settings.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/dots-spotifast-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
export HOME="$work/home with spaces" PATH="$work/bin:$PATH" TEST_LOG="$work/calls"
mkdir -p "$work/bin" "$work/Spotifast.app" "$HOME/.config/shell"

# Replace only the installed bundle location and PATH prefix in the fixture.
awk -v app="$work/Spotifast.app" '
  /^app=/ { print "app=\"" app "\""; next }
  /^export PATH=/ { next }
  { print }
' "$root/scripts/spotifast-launch.sh" > "$work/launch.sh"
cat > "$HOME/.config/shell/pi-proxy.sh" <<'SH'
_pi_proxy_ensure() {
  printf 'ensure %s\n' "$*" >> "$TEST_LOG"
  PI_PROXY_URL="http://127.0.0.1:${PI_PROXY_PORT:-10810}"
  return "${FAKE_PROXY_STATUS:-0}"
}
SH
cat > "$work/bin/pgrep" <<'SH'
#!/bin/sh
[ "$*" = '-x Spotifast' ] || exit 2
printf 'pgrep %s\n' "$*" >> "$TEST_LOG"
[ "${FAKE_RUNNING:-0}" = 1 ]
SH
cat > "$work/bin/open" <<'SH'
#!/bin/sh
printf 'open\n' >> "$TEST_LOG"
printf '%s\n' "$@" >> "$TEST_LOG"
SH
chmod +x "$work/bin/"*

check() {
  if ! "$@"; then printf 'Spotifast assertion failed: %s\n' "$*" >&2; exit 1; fi
}
expect_status() {
  local wanted=$1 actual=0
  shift
  "$@" > "$work/output" 2>&1 || actual=$?
  check test "$actual" -eq "$wanted"
}
: > "$TEST_LOG"
export FAKE_RUNNING=0
bash "$work/launch.sh"
check grep -Fxq 'ensure spotifast' "$TEST_LOG"
check grep -Fxq "$work/Spotifast.app" "$TEST_LOG"
for key in HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy; do
  check grep -Fxq "$key=http://127.0.0.1:10810" "$TEST_LOG"
done
check grep -Fxq 'NO_PROXY=127.0.0.1,localhost,::1,*.local' "$TEST_LOG"
check grep -Fxq 'no_proxy=127.0.0.1,localhost,::1,*.local' "$TEST_LOG"
: > "$TEST_LOG"
export PI_PROXY_PORT=11810
bash "$work/launch.sh"
check grep -Fxq 'HTTPS_PROXY=http://127.0.0.1:11810' "$TEST_LOG"
printf 'PASS new launches receive the shared proxy and local-network bypass\n'

: > "$TEST_LOG"
export FAKE_RUNNING=1
bash "$work/launch.sh"
check grep -Fxq 'open' "$TEST_LOG"
check grep -Fxq "$work/Spotifast.app" "$TEST_LOG"
check test "$(grep -c -- '--env' "$TEST_LOG")" -eq 0
printf 'PASS an already running app is reopened without restarting it\n'

: > "$TEST_LOG"
export FAKE_PROXY_STATUS=7
expect_status 7 bash "$work/launch.sh"
check test "$(grep -c '^open$' "$TEST_LOG")" -eq 0
unset FAKE_PROXY_STATUS
mv "$work/Spotifast.app" "$work/missing.app"
: > "$TEST_LOG"
expect_status 1 bash "$work/launch.sh"
check grep -Fq 'brew install --cask crmne/tap/spotifast' "$work/output"
check test ! -s "$TEST_LOG"
printf 'PASS missing apps and proxy failures do not launch anything\n'

check grep -Fq '    ~/.local/bin/spotifast-app:' "$root/steps/terminal.yml"
check test -x "$root/scripts/spotifast-launch.sh"
check grep -Fq 'path: scripts/spotifast-launch.sh' "$root/steps/terminal.yml"
check grep -Fq 'cask "crmne/tap/spotifast"' "$root/Brewfile"
printf 'PASS package and launcher link consistently target Spotifast\n'

check grep -Fxq '  { app_id = "rocks.spotifast.Spotifast", workspace = "2" },' "$root/config/rift/config.toml"
aerospace_rule=$(awk '/^if.app-id = '\''rocks.spotifast.Spotifast'\''$/ { getline; print }' "$root/config/aerospace/aerospace.toml")
check test "$aerospace_rule" = "run = 'move-node-to-workspace 2'"
for config in config/aerospace/aerospace.toml config/rift/config.toml; do
  check test "$(grep -c 'rocks.spotifast.Spotifast' "$root/$config")" -eq 1
  check test "$(grep -Ec '^[^#]*(alt-shift-space|Alt \+ Shift \+ Space)' "$root/$config" || true)" -eq 0
done
printf 'PASS Spotifast is managed on workspace 2 in both window managers\n'

for path in scripts/spotifast-toggle.sh scripts/spotifast-toggle.swift scripts/spotifast-aerospace-follow.sh tests/test-spotifast-toggle-live.sh; do
  check test ! -e "$root/$path"
done
for config in config/aerospace/aerospace.toml config/rift/config.toml steps/terminal.yml; do
  check test "$(grep -Ec 'spotifast-(toggle|aerospace-follow)' "$root/$config" || true)" -eq 0
done
printf 'PASS retired hide/show and workspace-follow helpers have no scripts, links, or hooks\n'
