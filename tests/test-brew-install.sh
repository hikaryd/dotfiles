#!/usr/bin/env bash
# No real Homebrew, downloads, applications or developer-tool installation.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/dots-brew-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/repo/scripts" "$work/bin" "$work/apps" "$work/state"
cp "$root/scripts/brew-install.sh" "$work/repo/scripts/"
cat > "$work/repo/Brewfile" <<'BREW'
tap "owner/tools"
brew "owner/tools/window-manager"
cask "owner/tools/launcher"
cask "mos"
cask "duongductrong/snapzy/snapzy"
BREW
cat > "$work/bin/brew" <<'SH'
#!/bin/bash
printf '%s\n' "$*" >> "$TEST_LOG"
case $1 in
  list) [[ -f "$TEST_STATE/$3" ]] ;;
  uninstall) rm -f "$TEST_STATE/${3##*/}" ;;
  bundle) exit "${FAKE_BUNDLE_STATUS:-0}" ;;
  trust) exit "${FAKE_TRUST_STATUS:-0}" ;;
  tap) exit 0 ;;
  *) exit 99 ;;
esac
SH
cat > "$work/bin/uname" <<'SH'
#!/bin/sh
printf 'Darwin\n'
SH
cat > "$work/bin/xcode-select" <<'SH'
#!/bin/sh
exit "${FAKE_TOOLS_STATUS:-0}"
SH
cp "$work/bin/xcode-select" "$work/bin/xcrun"
chmod +x "$work/bin/"* "$work/repo/scripts/brew-install.sh"
export PATH="$work/bin:$PATH" TEST_LOG="$work/calls" TEST_STATE="$work/state" DOTS_APPLICATIONS_DIR="$work/apps"
runner="$work/repo/scripts/brew-install.sh"
expect_status() {
  local want=$1 got=0
  "$runner" > "$work/output" 2>&1 || got=$?
  [[ $got -eq $want ]] || { /bin/cat "$work/output" >&2; printf 'Expected %s, got %s\n' "$want" "$got" >&2; exit 1; }
}
: > "$TEST_LOG"
export FAKE_TOOLS_STATUS=1
expect_status 1
grep -q 'xcode-select --install' "$work/output"
[[ ! -s "$TEST_LOG" ]]
printf 'PASS missing developer tools fail before package mutation\n'
unset FAKE_TOOLS_STATUS
for cask in mos snapzy; do touch "$TEST_STATE/$cask"; done
expect_status 0
grep -q '^uninstall --cask mos --force$' "$TEST_LOG"
grep -q '^uninstall --cask duongductrong/snapzy/snapzy --force$' "$TEST_LOG"
if grep -q -- '--zap' "$TEST_LOG"; then exit 1; fi
grep -q '^trust --formula owner/tools/window-manager$' "$TEST_LOG"
grep -q '^trust --cask owner/tools/launcher$' "$TEST_LOG"
if grep -q '^trust owner/tools$' "$TEST_LOG"; then exit 1; fi
grep -q '^bundle --file Brewfile$' "$TEST_LOG"
printf 'PASS stale casks are reconciled without deleting user preferences\n'
printf 'PASS trust is scoped to declared packages rather than entire taps\n'
: > "$TEST_LOG"
for cask in mos snapzy; do touch "$TEST_STATE/$cask"; done
mkdir -p "$work/apps/Mos.app" "$work/apps/Snapzy.app"
expect_status 0
if grep -q '^uninstall ' "$TEST_LOG"; then exit 1; fi
printf 'PASS healthy applications are left alone\n'
: > "$TEST_LOG"
export FAKE_TRUST_STATUS=19
expect_status 19
if grep -q '^bundle ' "$TEST_LOG"; then exit 1; fi
unset FAKE_TRUST_STATUS
printf 'PASS a trust failure is not silently ignored\n'
export FAKE_BUNDLE_STATUS=23
expect_status 23
grep -q 'Homebrew bundle failed (exit 23)' "$work/output"
printf 'PASS the real bundle failure status reaches the installer\n'
