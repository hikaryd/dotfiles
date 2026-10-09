#!/usr/bin/env bash
# Run owned suites only; never recurse into Dotbot, caches or private configs.
set -uo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd) || exit 1
cd "$root" || exit 1

usage() {
  printf '%s\n' \
    'Usage: scripts/test.sh [all|go|python|node|runner|--list]' \
    'Defaults to all. PYTHON_BIN, GO_BIN and NODE_BIN may select local runtimes.'
}
if [[ $# -gt 1 ]]; then
  usage >&2
  exit 2
fi
group=${1:-all}
case "$group" in
  all|go|python|node|runner) ;;
  --list)
    printf '%s\n' \
      'runner: tests/test-runner.sh (isolated fake runtimes)' \
      'go: tools/*/go.mod (go test -race and go vet; untested modules stay visible)' \
      'python: scripts/tests, config/herdr/plugins/task-titles, vpn/tests' \
      'node: config/pi/tests/*.test.mjs'
    exit 0
    ;;
  -h|--help) usage; exit 0 ;;
  *) usage >&2; exit 2 ;;
esac

umask 077
logs=$(mktemp -d "${TMPDIR:-/tmp}/dots-tests.XXXXXX") || exit 1
trap 'rm -rf "$logs"' EXIT
mkdir -p "$logs/home" "$logs/config" "$logs/state" "$logs/data" || exit 1
failures=0
summary=()

run_suite() {
  local name=$1 status
  shift
  printf '\n== %s ==\n' "$name"
  if "$@"; then
    summary+=("PASS $name")
  else
    status=$?
    summary+=("FAIL $name (exit $status)")
    failures=$((failures + 1))
  fi
}

python_suite() {
  local directory=$1 log=$2
  local -a statuses
  # generate.py reads VPN_ENV at import time. Never let tests import the real
  # VPS subscription/credentials, even when the caller exported VPN_ENV.
  HOME="$logs/home" XDG_CONFIG_HOME="$logs/config" \
    XDG_STATE_HOME="$logs/state" XDG_DATA_HOME="$logs/data" \
    VPN_ENV=/dev/null PYTHONDONTWRITEBYTECODE=1 \
    "${PYTHON_BIN:-python3}" -m unittest discover \
    -s "$directory" -p 'test_*.py' -v 2>&1 | tee "$log"
  statuses=("${PIPESTATUS[@]}")
  if [[ ${statuses[0]} -ne 0 ]]; then return "${statuses[0]}"; fi
  if [[ ${statuses[1]} -ne 0 ]]; then return "${statuses[1]}"; fi
  if ! grep -Eq '^Ran [1-9][0-9]* tests? in ' "$log"; then
    printf 'No tests collected in %s; refusing a false-green result.\n' "$directory" >&2
    return 1
  fi
}

node_suite() {
  local log=$1
  local -a files statuses
  files=(config/pi/tests/*.test.mjs)
  if [[ ! -f ${files[0]} ]]; then
    printf 'No Node test files found.\n' >&2
    return 1
  fi
  "${NODE_BIN:-node}" --test --test-reporter=tap "${files[@]}" 2>&1 | tee "$log"
  statuses=("${PIPESTATUS[@]}")
  if [[ ${statuses[0]} -ne 0 ]]; then return "${statuses[0]}"; fi
  if [[ ${statuses[1]} -ne 0 ]]; then return "${statuses[1]}"; fi
  if ! grep -Eq '^# tests [1-9][0-9]*$' "$log"; then
    printf 'No Node tests collected; refusing a false-green result.\n' >&2
    return 1
  fi
}

if [[ $group == all || $group == runner ]]; then
  run_suite 'runner contract' bash tests/test-runner.sh
fi
if [[ $group == all || $group == go ]]; then
  modules=(tools/*/go.mod)
  if [[ ! -f ${modules[0]} ]]; then
    run_suite 'Go modules' false
  else
    for manifest in "${modules[@]}"; do
      module=${manifest%/go.mod}
      if [[ -n $(find "$module" -name '*_test.go' -print -quit) ]]; then
        label="$module / race"
      else
        label="$module / compile (no tests)"
      fi
      # Use the installed toolchain; testing must not auto-download another Go.
      run_suite "$label" env GOTOOLCHAIN=local \
        "${GO_BIN:-go}" -C "$module" test -race -count=1 ./...
      run_suite "$module / vet" env GOTOOLCHAIN=local \
        "${GO_BIN:-go}" -C "$module" vet ./...
    done
  fi
fi
if [[ $group == all || $group == python ]]; then
  run_suite 'Python / installers' python_suite scripts/tests "$logs/installers.log"
  run_suite 'Python / Herdr titles' python_suite config/herdr/plugins/task-titles "$logs/titles.log"
  run_suite 'Python / VPN' python_suite vpn/tests "$logs/vpn.log"
fi
if [[ $group == all || $group == node ]]; then
  run_suite 'Node / status bar' node_suite "$logs/node.log"
fi

printf '\nTest summary:\n'
printf '%s\n' "${summary[@]}"
printf 'Failed suites: %s\n' "$failures"
[[ $failures -eq 0 ]]
