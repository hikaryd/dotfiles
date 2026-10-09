#!/usr/bin/env bash
# Test orchestration with fake runtimes: no real installs, network or user data.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
runner="$root/scripts/test.sh"
work=$(mktemp -d "${TMPDIR:-/tmp}/dots-runner-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"

# Fixtures record arguments and emit the native frameworks' summary formats.
cat > "$work/bin/python" <<'SH'
#!/bin/sh
printf 'python %s\n' "$*" >> "$TEST_LOG"
printf 'HOME=%s VPN_ENV=%s\n' "$HOME" "$VPN_ENV" >> "$TEST_LOG"
if [ "${FAKE_FAIL_INSTALLERS:-}" = 1 ]; then
  case "$*" in *scripts/tests*) exit 23 ;; esac
fi
if [ "${FAKE_EMPTY:-}" = python ]; then
  printf '\nRan 0 tests in 0.001s\n\nOK\n' >&2
else
  printf '\nRan 2 tests in 0.001s\n\nOK\n' >&2
fi
SH
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
printf 'ok fixture\n'
exit "${FAKE_GO_STATUS:-0}"
SH
chmod +x "$work/bin/"*

export TEST_LOG="$work/calls"
export PYTHON_BIN="$work/bin/python" NODE_BIN="$work/bin/node" GO_BIN="$work/bin/go"
export VPN_ENV="$work/must-not-read-live-vpn.env"

check() {
  if ! "$@"; then
    printf 'Runner assertion failed: %s\n' "$*" >&2
    exit 1
  fi
}
reset() {
  : > "$TEST_LOG"
  unset FAKE_EMPTY FAKE_FAIL_INSTALLERS FAKE_NODE_STATUS FAKE_GO_STATUS
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
expect_status 0 python
check test "$(grep -c '^python ' "$TEST_LOG")" -eq 3
check grep -q -- '-m unittest discover' "$TEST_LOG"
check grep -q 'VPN_ENV=/dev/null' "$TEST_LOG"
check grep -q 'HOME=.*/dots-tests\..*/home' "$TEST_LOG"
if grep -Eq '^(go|node) ' "$TEST_LOG"; then exit 1; fi
printf 'PASS Python discovery is explicit, isolated and group-scoped\n'

reset
export FAKE_EMPTY=python
expect_status 1 python
check grep -q 'No tests collected' "$work/output"
check grep -q 'Failed suites: 3' "$work/output"
printf 'PASS zero Python tests fail instead of passing silently\n'

reset
export FAKE_FAIL_INSTALLERS=1
expect_status 1 python
check test "$(grep -c '^python ' "$TEST_LOG")" -eq 3
check grep -q 'FAIL Python / installers (exit 23)' "$work/output"
check grep -q 'PASS Python / VPN' "$work/output"
printf 'PASS a failed suite does not hide its failure or prevent later suites\n'

reset
expect_status 0 node
check grep -q -- '--test --test-reporter=tap' "$TEST_LOG"
if grep -Eq '^(go|python) ' "$TEST_LOG"; then exit 1; fi
printf 'PASS Node uses its native test runner\n'

reset
export FAKE_EMPTY=node
expect_status 1 node
check grep -q 'No Node tests collected' "$work/output"
printf 'PASS zero Node tests fail instead of passing silently\n'

reset
export FAKE_NODE_STATUS=17
expect_status 1 node
check grep -q 'exit 17' "$work/output"
printf 'PASS native test failures propagate even with nonempty output\n'

reset
expect_status 0 go
check grep -q -- 'test -race -count=1 ./...' "$TEST_LOG"
check grep -q -- 'vet ./...' "$TEST_LOG"
if grep -Eq '^(node|python) ' "$TEST_LOG"; then exit 1; fi
printf 'PASS Go modules use race tests without result caching and run vet\n'

reset
export FAKE_GO_STATUS=29
expect_status 1 go
check grep -q 'exit 29' "$work/output"
printf 'PASS Go failures propagate\n'

reset
export PYTHON_BIN="$work/not-installed"
expect_status 1 python
check grep -q 'exit 127' "$work/output"
printf 'PASS missing runtimes fail explicitly\n'

reset
expect_status 2 invalid
expect_status 2 python node
check test ! -s "$TEST_LOG"
(cd "$work" && "$runner" --list > "$work/output")
check grep -q '^python:' "$work/output"
check test ! -s "$TEST_LOG"
printf 'PASS usage/list do not execute suites and work outside the repo root\n'
