#!/usr/bin/env bash
# Fake-runtime contracts: no real installs, network or user data.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
runner="$root/scripts/test.sh"
work=$(mktemp -d "${TMPDIR:-/tmp}/dots-runner-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"
cat > "$work/bin/node" <<'SH'
#!/bin/sh
printf 'node %s\n' "$*" >> "$TEST_LOG"
if [ "${FAKE_EMPTY:-}" = node ]; then
  printf 'TAP version 13\n1..0\n# tests 0\n# fail 0\n'
else
  printf 'TAP version 13\n1..1\nok 1 - fixture\n# tests 1\n# fail 0\n'
fi
exit "${FAKE_NODE_STATUS:-0}"
SH
cat > "$work/bin/go" <<'SH'
#!/bin/sh
printf 'go %s\n' "$*" >> "$TEST_LOG"
case "$*" in
  *tools/dots\ test*) exit "${FAKE_GO_STATUS:-0}" ;;
esac
printf 'ok fixture\n'
SH
chmod +x "$work/bin/"*
export TEST_LOG="$work/calls" NODE_BIN="$work/bin/node" GO_BIN="$work/bin/go"
check() {
  if ! "$@"; then printf 'Runner assertion failed: %s\n' "$*" >&2; exit 1; fi
}
reset() {
  : > "$TEST_LOG"
  unset FAKE_EMPTY FAKE_NODE_STATUS FAKE_GO_STATUS
}
expect_status() {
  local wanted=$1 actual=0
  shift
  "$runner" "$@" > "$work/output" 2>&1 || actual=$?
  if [[ $actual -ne $wanted ]]; then
    printf 'Runner exit %s, expected %s\n' "$actual" "$wanted" >&2
    /bin/cat "$work/output" >&2
    exit 1
  fi
}
reset
expect_status 0 node
check grep -q -- '--test --test-reporter=tap' "$TEST_LOG"
if grep -q '^go ' "$TEST_LOG"; then exit 1; fi
printf 'PASS Node uses its native runner and is group-scoped\n'
reset
export FAKE_EMPTY=node
expect_status 1 node
check grep -q 'No Node tests collected' "$work/output"
printf 'PASS empty Node suites fail instead of passing silently\n'
reset
export FAKE_NODE_STATUS=17
expect_status 1 node
check grep -q 'exit 17' "$work/output"
printf 'PASS native failures propagate even with nonempty output\n'
reset
expect_status 0 go
check grep -q -- 'test -race -count=1 ./...' "$TEST_LOG"
check grep -q -- 'vet ./...' "$TEST_LOG"
check grep -q -- 'tools/dots test' "$TEST_LOG"
check grep -q -- 'compile (no tests)' "$work/output"
if grep -q '^node ' "$TEST_LOG"; then exit 1; fi
printf 'PASS Go discovery includes the native runtime, race and vet\n'
reset
export FAKE_GO_STATUS=29
expect_status 1 go
check grep -q 'FAIL tools/dots / race (exit 29)' "$work/output"
check grep -q 'PASS tools/herdr-last-workspace / race' "$work/output"
check grep -q 'PASS tools/dots / vet' "$work/output"
printf 'PASS a failure is reported without preventing later suites\n'
reset
export GO_BIN="$work/not-installed"
expect_status 1 go
check grep -q 'exit 127' "$work/output"
printf 'PASS missing runtimes fail explicitly\n'
reset
expect_status 2 invalid
expect_status 2 go node
expect_status 2 python
check test ! -s "$TEST_LOG"
(cd "$work" && "$runner" --list > "$work/output")
check grep -q '^go:' "$work/output"
check grep -q '^node:' "$work/output"
if grep -q '^python:' "$work/output"; then exit 1; fi
check test ! -s "$TEST_LOG"
printf 'PASS help/list work outside the checkout without executing suites\n'
