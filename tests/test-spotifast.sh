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
