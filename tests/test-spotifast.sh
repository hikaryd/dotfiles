#!/usr/bin/env bash
# Isolated contracts: no real apps, proxy processes, or user settings.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/dots-spotifast-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
export HOME="$work/home with spaces" PATH="$work/bin:$PATH" TEST_LOG="$work/calls"
mkdir -p "$work/bin" "$work/Spotifast.app" "$HOME/.config/shell" "$HOME/.local/bin" "$HOME/.cache/dots"

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
cat > "$work/bin/aerospace" <<'SH'
#!/bin/sh
printf 'aerospace %s\n' "$*" >> "$TEST_LOG"
case "$1" in
  list-workspaces) printf '%s\n' "${FAKE_WORKSPACE:-w}" ;;
  list-windows)
    printf '%s\n' '41|rocks.spotifast.Spotifast|1' '42|rocks.spotifast.Spotifast|w' \
      '43|other.client|1' '44|rocks.spotifast.Spotifast|2' ;;
esac
SH
cat > "$HOME/.local/bin/spotifast-app" <<'SH'
#!/bin/sh
printf 'launcher\n' >> "$TEST_LOG"
SH
cp "$root/scripts/spotifast-toggle.sh" "$root/scripts/spotifast-toggle.swift" "$work/"
cat > "$HOME/.cache/dots/spotifast-toggle" <<'SH'
#!/bin/sh
printf '%s\n' "$FAKE_ACTION"
SH
chmod +x "$work/bin/"* "$HOME/.local/bin/spotifast-app" "$HOME/.cache/dots/spotifast-toggle"
# The cached fixture must be newer than the source so the test never runs swiftc.
touch -t 203001010000 "$HOME/.cache/dots/spotifast-toggle"

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

: > "$TEST_LOG"
export AEROSPACE_FOCUSED_WORKSPACE=w
bash "$root/scripts/spotifast-aerospace-follow.sh"
check grep -Fxq 'aerospace move-node-to-workspace --window-id 41 w' "$TEST_LOG"
check grep -Fxq 'aerospace move-node-to-workspace --window-id 44 w' "$TEST_LOG"
check test "$(grep -c '^aerospace move-node' "$TEST_LOG")" -eq 2
: > "$TEST_LOG"
unset AEROSPACE_FOCUSED_WORKSPACE
bash "$root/scripts/spotifast-aerospace-follow.sh"
check grep -Fxq 'aerospace list-workspaces --focused' "$TEST_LOG"
: > "$TEST_LOG"
export FAKE_RUNNING=0
bash "$root/scripts/spotifast-aerospace-follow.sh"
check test "$(grep -c '^aerospace' "$TEST_LOG")" -eq 0
printf 'PASS only Spotifast windows follow the focused AeroSpace workspace\n'

for action in launch shown hidden; do
  : > "$TEST_LOG"
  export FAKE_ACTION="$action"
  bash "$work/spotifast-toggle.sh" --locked
  if [[ "$action" == hidden ]]; then
    check test ! -s "$TEST_LOG"
  else
    check grep -Fxq launcher "$TEST_LOG"
  fi
done
printf 'PASS the toggle launches or restores Spotifast, but keeps it hidden on hide\n'

for name in spotifast-app spotifast-aerospace-follow spotifast-toggle; do
  check grep -Fq "    ~/.local/bin/$name:" "$root/steps/terminal.yml"
done
for script in launch aerospace-follow toggle; do
  check test -x "$root/scripts/spotifast-$script.sh"
  check grep -Fq "path: scripts/spotifast-$script.sh" "$root/steps/terminal.yml"
done
check grep -Fq 'cask "crmne/tap/spotifast"' "$root/Brewfile"
for config in config/aerospace/aerospace.toml config/rift/config.toml scripts/spotifast-toggle.swift; do
  check grep -Fq 'rocks.spotifast.Spotifast' "$root/$config"
done
printf 'PASS package, links, and window rules consistently target Spotifast\n'
