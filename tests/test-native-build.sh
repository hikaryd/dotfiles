#!/usr/bin/env bash
# Isolated ownership contracts. No real builds, installs or user configuration.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/dots-native-build-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin" "$work/repo/scripts" "$work/home" "$work/data"
cp "$root/scripts/build-go-tools.sh" "$root/scripts/terminal-ui.sh" "$work/repo/scripts/"
cp "$root/uninstall" "$work/repo/uninstall"
export HOME="$work/home" XDG_DATA_HOME="$work/data" XDG_CACHE_HOME="$work/cache" XDG_CONFIG_HOME="$work/config"
export NATIVE_BUILD_CALLS="$work/build-calls" NATIVE_RUNTIME_CALLS="$work/runtime-calls"
export GIT_CONFIG_GLOBAL="$work/gitconfig" GIT_CONFIG_NOSYSTEM=1
export PATH="$work/bin:$PATH" NO_COLOR=1
: > "$NATIVE_BUILD_CALLS"
: > "$NATIVE_RUNTIME_CALLS"
cat > "$work/bin/go" <<'SH'
#!/bin/sh
printf '%s\n' "$*" >> "$NATIVE_BUILD_CALLS"
[ "${FAKE_BUILD_FAILURE:-0}" = 0 ] || exit "$FAKE_BUILD_FAILURE"
output=''
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then output=$2; shift; fi
  shift
done
[ -n "$output" ] || exit 97
cat > "$output" <<'BIN'
#!/bin/sh
printf '%s\n' "$*" >> "$NATIVE_RUNTIME_CALLS"
case "$*" in
  commands) printf 'kpdev\n' ;;
  'kube commands') printf 'kpdev\n' ;;
  'setup managed-links '*) printf '~/.local/bin/dots-tools\n' ;;
  ui\ *) cat ;;
esac
BIN
SH
cat > "$work/bin/dscl" <<'SH'
#!/bin/sh
printf 'UserShell: /bin/zsh\n'
SH
chmod +x "$work/bin/go" "$work/bin/dscl"
check() {
  if ! "$@"; then printf 'Native ownership assertion failed: %s\n' "$*" >&2; exit 1; fi
}
expect_status() {
  local expected=$1 actual=0
  shift
  "$@" > "$work/output" 2>&1 || actual=$?
  if [[ $actual -ne $expected ]]; then
    cat "$work/output" >&2
    printf 'Expected exit %s, got %s: %s\n' "$expected" "$actual" "$*" >&2
    exit 1
  fi
}
build_contract() {
  expect_status 0 bash "$work/repo/scripts/build-go-tools.sh"
  if grep -q 'cmd/dots-kube' "$NATIVE_BUILD_CALLS"; then
    printf 'Dots still builds the retired standalone runtime\n' >&2
    exit 1
  fi
  check test "$(wc -l < "$NATIVE_BUILD_CALLS" | tr -d ' ')" = 1
  check test ! -e "$work/data/dots/bin/dots-kube"
  for name in dots dots-nu ai_helper dots-vpn; do
    check test "$(readlink "$work/repo/build/bin/$name")" = dots-tools
  done
  for name in pi-install dots-task-titles; do
    check test ! -e "$work/repo/build/bin/$name"
    check test ! -L "$work/repo/build/bin/$name"
    check test ! -e "$HOME/.local/bin/$name"
  done
  printf 'PASS only the remaining dots runtime is built\n'

  printf 'standalone executable\n' > "$HOME/.local/bin/kubedesk"
  printf 'retired local command, not owned by this build\n' > "$HOME/.local/bin/dots-kube"
  cp "$HOME/.local/bin/kubedesk" "$work/standalone-before"
  cp "$HOME/.local/bin/dots-kube" "$work/retired-before"
  ln -s kubedesk "$HOME/.local/bin/kpdev"
  ln -s kubedesk "$HOME/.local/bin/km"
  ln -s standalone-owned "$work/repo/build/bin/kpdev"
  for attempt in 1 2; do
    expect_status 0 bash "$work/repo/scripts/build-go-tools.sh"
    check cmp "$work/standalone-before" "$HOME/.local/bin/kubedesk"
    check cmp "$work/retired-before" "$HOME/.local/bin/dots-kube"
    check test "$(readlink "$HOME/.local/bin/kpdev")" = kubedesk
    check test "$(readlink "$HOME/.local/bin/km")" = kubedesk
    check test "$(readlink "$work/repo/build/bin/kpdev")" = standalone-owned
  done
  printf 'PASS repeated rebuilds leave standalone binaries and aliases untouched\n'

  cp "$work/data/dots/bin/dots-tools" "$work/runtime-before"
  export FAKE_BUILD_FAILURE=23
  expect_status 23 bash "$work/repo/scripts/build-go-tools.sh"
  unset FAKE_BUILD_FAILURE
  check cmp "$work/runtime-before" "$work/data/dots/bin/dots-tools"
  printf 'PASS compiler failures preserve the previous working runtime\n'

  rm "$HOME/.local/bin/dots-vpn"
  printf 'foreign command\n' > "$HOME/.local/bin/dots-vpn"
  expect_status 1 bash "$work/repo/scripts/build-go-tools.sh"
  check grep -Fxq 'foreign command' "$HOME/.local/bin/dots-vpn"
  check grep -q 'Existing command preserved' "$work/output"
  printf 'PASS foreign commands are not overwritten\n'
}
uninstall_contract() {
  mkdir -p "$work/repo/build/bin" "$HOME/.local/bin"
  "$work/bin/go" build -o "$work/repo/build/bin/dots-tools"
  chmod +x "$work/repo/build/bin/dots-tools"
  if [[ ! -L "$HOME/.local/bin/kpdev" ]]; then
    ln -s "$work/repo/build/bin/kpdev" "$HOME/.local/bin/kpdev"
  fi
  : > "$NATIVE_RUNTIME_CALLS"
  expect_status 0 bash "$work/repo/uninstall" --dry-run
  if grep -q '^kube ' "$NATIVE_RUNTIME_CALLS"; then
    printf 'Dots uninstall still requests the retired command inventory\n' >&2
    exit 1
  fi
  if grep -q 'kpdev' "$work/output"; then
    printf 'Dots uninstall still plans removal of standalone aliases\n' >&2
    exit 1
  fi
  check test -L "$HOME/.local/bin/kpdev"
  printf 'PASS uninstall does not own standalone aliases or require their runtime\n'
}
case ${1:-all} in
  all) build_contract; uninstall_contract ;;
  build) build_contract ;;
  uninstall) uninstall_contract ;;
  *) printf 'Usage: test-native-build.sh [all|build|uninstall]\n' >&2; exit 2 ;;
esac
