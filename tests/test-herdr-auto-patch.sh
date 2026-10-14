#!/usr/bin/env bash
# Isolated launcher/build contracts: no real downloads, compilation or sessions.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/dots-herdr-auto-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/repo/scripts" "$work/repo/patches/herdr" "$work/bin" "$work/links"
cp "$root/scripts/herdr" "$root/scripts/build-herdr-ui.sh" "$work/repo/scripts/"
cp "$root/patches/herdr/"*-centered-rounded-tabs.patch "$work/repo/patches/herdr/"
ln -s "$work/repo/scripts/herdr" "$work/links/herdr"
cat > "$work/bin/base" <<'SH'
#!/bin/sh
if [ "${1:-}" = --version ]; then echo "herdr $FAKE_BASE_VERSION"; else printf 'BASE %s\n' "$*"; fi
SH
cat > "$work/bin/zig" <<'SH'
#!/bin/sh
printf '%s\n' "${FAKE_SYSTEM_ZIG_VERSION:-0.16.0}"
SH
cat > "$work/bin/git" <<'SH'
#!/bin/bash
printf 'git %s\n' "$*" >> "$TEST_LOG"
case $1 in
  clone) mkdir -p "${@: -1}" ;;
  rev-parse) printf '%s\n' "${FAKE_COMMIT:-7e26bad94044eba205033111a0ce8033ede7e24f}" ;;
  apply) if [[ ${2:-} == --check && ${FAKE_PATCH_FAIL:-0} == 1 ]]; then echo 'fixture: incompatible patch' >&2; exit 3; fi ;;
  *) exit 99 ;;
esac
SH
cat > "$work/bin/cargo" <<'SH'
#!/bin/bash
printf 'cargo %s\n' "$*" >> "$TEST_LOG"
[[ ${FAKE_CARGO_FAIL:-0} != 1 ]] || exit 4
case $1 in
  test) echo "test result: ok. ${FAKE_TEST_COUNT:-1} passed; 0 failed" ;;
  build)
    sleep "${FAKE_BUILD_DELAY:-0}"
    mkdir -p "$CARGO_TARGET_DIR/release"
    printf '#!/bin/sh\nif [ "${1:-}" = --version ]; then echo "herdr %s"; else printf "PATCHED %%s\\n" "$*"; fi\n' "${FAKE_BUILT_VERSION:-$FAKE_BASE_VERSION}" > "$CARGO_TARGET_DIR/release/herdr"
    chmod +x "$CARGO_TARGET_DIR/release/herdr"
    ;;
  *) exit 99 ;;
esac
SH
cat > "$work/bin/curl" <<'SH'
#!/bin/bash
printf 'curl %s\n' "$*" >> "$TEST_LOG"
[[ ${FAKE_DOWNLOAD_FAIL:-0} != 1 ]] || exit 5
while [[ $# -gt 0 ]]; do
  if [[ $1 == -o ]]; then touch "$2"; shift 2; else shift; fi
 done
SH
cat > "$work/bin/shasum" <<'SH'
#!/bin/bash
if [[ $* == *zig.tar.xz* || $* == *' -c '* ]]; then
  printf 'checksum %s\n' "$*" >> "$TEST_LOG"
  [[ ${FAKE_CHECKSUM_FAIL:-0} != 1 ]] || exit 6
  if [[ $* == *' -c '* ]]; then while read -r line; do :; done; else printf 'b23d70deaa879b5c2d486ed3316f7eaa53e84acf6fc9cc747de152450d401489  %s\n' "${@: -1}"; fi
else
  exec /usr/bin/shasum "$@"
fi
SH
cat > "$work/bin/tar" <<'SH'
#!/bin/bash
printf 'tar %s\n' "$*" >> "$TEST_LOG"
while [[ $# -gt 0 ]]; do
  if [[ $1 == -C ]]; then destination=$2; shift 2; else shift; fi
 done
mkdir -p "$destination/zig-aarch64-macos-0.16.0"
printf '#!/bin/sh\necho 0.16.0\n' > "$destination/zig-aarch64-macos-0.16.0/zig"
chmod +x "$destination/zig-aarch64-macos-0.16.0/zig"
SH
cat > "$work/bin/uname" <<'SH'
#!/bin/sh
case $1 in -s) echo Darwin ;; -m) echo arm64 ;; *) exit 99 ;; esac
SH
chmod +x "$work/bin/"*
export PATH="$work/bin:$PATH" HERDR_DOTS_BASE="$work/bin/base" TEST_LOG="$work/calls"
export XDG_DATA_HOME="$work/data" XDG_CACHE_HOME="$work/cache" FAKE_BASE_VERSION=0.9.4
unset ZIG CARGO_TARGET_DIR HERDR_DOTS_NO_PATCH
runner="$work/links/herdr"
builder="$work/repo/scripts/build-herdr-ui.sh"
state="$XDG_DATA_HOME/dots/herdr"
reset() {
  rm -rf "$XDG_DATA_HOME" "$XDG_CACHE_HOME"
  : > "$TEST_LOG"
  unset FAKE_PATCH_FAIL FAKE_CARGO_FAIL FAKE_SYSTEM_ZIG_VERSION FAKE_DOWNLOAD_FAIL FAKE_CHECKSUM_FAIL FAKE_BUILT_VERSION FAKE_COMMIT FAKE_BUILD_DELAY FAKE_TEST_COUNT
  export FAKE_BASE_VERSION=0.9.4
}
expect_output() {
  local expected=$1
  shift
  "$runner" "$@" > "$work/output" 2> "$work/error" || { printf 'Launcher failed before %s:\n' "$expected" >&2; /bin/cat "$work/output" "$work/error" >&2; exit 1; }
  grep -q "$expected" "$work/output" || { printf 'Expected %s, got:\n' "$expected" >&2; /bin/cat "$work/output" "$work/error" >&2; exit 1; }
}
count() { grep -c "$1" "$TEST_LOG" || true; }
reset
expect_output '^PATCHED '
[[ $(count '^git clone') == 1 && $(count '^cargo build') == 1 ]]
expect_output '^PATCHED '
[[ $(count '^git clone') == 1 ]]
printf 'PASS symlink launcher builds once and reuses a version-matched cache\n'
reset
for args in '--version' '--help' 'status client' 'server reload-config' 'update' '--session work status client' '--machine remote status'; do
  read -r -a argv <<< "$args"
  "$runner" "${argv[@]}" > "$work/output"
done
[[ ! -s "$TEST_LOG" ]]
export HERDR_DOTS_NO_PATCH=1
expect_output '^BASE '
unset HERDR_DOTS_NO_PATCH
[[ ! -s "$TEST_LOG" ]]
printf 'PASS metadata, management commands and opt-out never start builds\n'
for shape in 'session attach work' '--session work' '--remote example' '--session work --handoff'; do
  reset
  read -r -a argv <<< "$shape"
  expect_output '^PATCHED ' "${argv[@]}"
  [[ $(count '^cargo build') == 1 ]]
done
printf 'PASS named sessions and remote clients get the local UI patch\n'
reset
export FAKE_BASE_VERSION=0.9.5 FAKE_COMMIT=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
expect_output '^PATCHED '
grep -q -- '--branch v0.9.5' "$TEST_LOG"
[[ $(< "$state/0.9.5/source-commit") == "$FAKE_COMMIT" ]]
printf 'PASS a new stable release tries the maintained patch rather than downgrading\n'
reset
expect_output '^PATCHED '
printf '\n' >> "$work/repo/patches/herdr/0.9.4-centered-rounded-tabs.patch"
expect_output '^PATCHED '
[[ $(count '^cargo build') == 2 ]]
printf 'PASS changing a patch invalidates the same-version cache\n'
reset
export FAKE_PATCH_FAIL=1
expect_output '^BASE '
grep -q 'build.log' "$work/error"
[[ $(count '^cargo ') == 0 ]]
expect_output '^BASE '
[[ $(count '^git clone') == 1 ]]
unset FAKE_PATCH_FAIL
"$builder" 0.9.4 > "$work/output"
expect_output '^PATCHED '
[[ $(count '^git clone') == 2 ]]
printf 'PASS incompatible patches fall back, throttle retries and allow manual recovery\n'
for variable in FAKE_CARGO_FAIL FAKE_TEST_COUNT FAKE_BUILT_VERSION FAKE_COMMIT; do
  reset
  case $variable in
    FAKE_CARGO_FAIL) export FAKE_CARGO_FAIL=1 ;;
    FAKE_TEST_COUNT) export FAKE_TEST_COUNT=0 ;;
    FAKE_BUILT_VERSION) export FAKE_BUILT_VERSION=0.9.3 ;;
    FAKE_COMMIT) export FAKE_COMMIT=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb ;;
  esac
  expect_output '^BASE '
  [[ ! -e "$state/0.9.4/herdr" ]]
done
printf 'PASS compilation, empty tests, binary version and reviewed-tag mismatches fail closed\n'
reset
expect_output '^PATCHED '
cp "$state/0.9.4/herdr" "$work/original"
printf '\n' >> "$work/repo/patches/herdr/0.9.4-centered-rounded-tabs.patch"
export FAKE_CARGO_FAIL=1
expect_output '^PATCHED '
cmp "$work/original" "$state/0.9.4/herdr"
printf 'PASS failed rebuilds preserve the last good client\n'
reset
export FAKE_SYSTEM_ZIG_VERSION=0.17.0
expect_output '^PATCHED '
[[ $(count '^curl ') == 1 && $(count '^tar ') == 1 && $(count '^checksum ') -ge 1 ]]
export FAKE_BASE_VERSION=0.9.5 FAKE_COMMIT=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
expect_output '^PATCHED '
[[ $(count '^curl ') == 1 ]]
printf 'PASS pinned Zig is downloaded, verified and reused without changing Homebrew\n'
for variable in FAKE_DOWNLOAD_FAIL FAKE_CHECKSUM_FAIL; do
  reset
  export FAKE_SYSTEM_ZIG_VERSION=0.17.0
  export "$variable=1"
  expect_output '^BASE '
  [[ $(count '^cargo ') == 0 && $(count '^tar ') == 0 ]]
done
printf 'PASS download and checksum failures never extract or compile\n'
reset
export FAKE_BUILD_DELAY=1
"$runner" > "$work/first" 2> "$work/first-error" & first=$!
"$runner" > "$work/second" 2> "$work/second-error" & second=$!
wait "$first"
wait "$second"
grep -q '^PATCHED ' "$work/first"
grep -q '^PATCHED ' "$work/second"
[[ $(count '^cargo build') == 1 && ! -e "$state/build.lock" ]]
printf 'PASS concurrent starts share one build and release the lock\n'
reset
mkdir -p "$state"
printf '999999999\n' > "$state/build.lock"
expect_output '^PATCHED '
[[ ! -e "$state/build.lock" ]]
printf 'PASS stale process locks recover after an interrupted build\n'
reset
export FAKE_BASE_VERSION='0.9.5/../../bad'
expect_output '^BASE '
[[ ! -s "$TEST_LOG" ]]
printf 'PASS malformed versions never become download refs or cache paths\n'
