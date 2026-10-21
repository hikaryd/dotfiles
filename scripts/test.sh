#!/usr/bin/env bash
# Native suites only; never recurse into Dotbot, caches or private configs.
set -uo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd) || exit 1
cd "$root" || exit 1
usage() {
  printf '%s\n' 'Usage: scripts/test.sh [all|go|node|runner|--list]' \
    'Defaults to all. GO_BIN and NODE_BIN select local runtimes.'
}
[[ $# -le 1 ]] || { usage >&2; exit 2; }
group=${1:-all}
case "$group" in
  all|go|node|runner) ;;
  --list)
    printf '%s\n' 'runner: tests/test-runner.sh, tests/test-pytest-safe.zsh, tests/test-spotifast.sh, tests/test-brew-install.sh, tests/test-herdr-auto-patch.sh (isolated shell contracts)' \
      'go: tools/*/go.mod (race tests and vet; untested modules stay visible)' \
      'node: tools/task-tab-groups/tests/*.test.js, tests/*.test.mjs'
    exit 0 ;;
  -h|--help) usage; exit 0 ;;
  *) usage >&2; exit 2 ;;
esac
umask 077
logs=$(mktemp -d "${TMPDIR:-/tmp}/dots-tests.XXXXXX") || exit 1
trap 'rm -rf "$logs"' EXIT
failures=0
summary=()
run_suite() {
  local name=$1 status
  shift
  printf '\n== %s ==\n' "$name"
  if "$@"; then summary+=("PASS $name"); else
    status=$?
    summary+=("FAIL $name (exit $status)")
    failures=$((failures + 1))
  fi
}
node_suite() {
  local -a files statuses
  shopt -s nullglob
  files=(tools/task-tab-groups/tests/*.test.js tests/*.test.mjs)
  shopt -u nullglob
  [[ -f ${files[0]:-} ]] || { printf 'No Node test files found.\n' >&2; return 1; }
  "${NODE_BIN:-node}" --test --test-reporter=tap "${files[@]}" 2>&1 | tee "$logs/node.log"
  statuses=("${PIPESTATUS[@]}")
  [[ ${statuses[0]} -eq 0 ]] || return "${statuses[0]}"
  [[ ${statuses[1]} -eq 0 ]] || return "${statuses[1]}"
  if ! grep -Eq '^# tests [1-9][0-9]*$' "$logs/node.log"; then
    printf 'No Node tests collected; refusing a false-green result.\n' >&2
    return 1
  fi
}
if [[ $group == all || $group == runner ]]; then
  run_suite 'runner contract' bash tests/test-runner.sh
  run_suite 'pytest-safe contract' zsh -f tests/test-pytest-safe.zsh
  run_suite 'Spotifast contract' bash tests/test-spotifast.sh
  run_suite 'Homebrew installer contract' bash tests/test-brew-install.sh
  run_suite 'Herdr automatic UI patch contract' bash tests/test-herdr-auto-patch.sh
fi
if [[ $group == all || $group == go ]]; then
  modules=(tools/*/go.mod)
  if [[ ! -f ${modules[0]} ]]; then run_suite 'Go modules' false; else
    for manifest in "${modules[@]}"; do
      module=${manifest%/go.mod}
      if [[ -n $(find "$module" -name '*_test.go' -print -quit) ]]; then
        label="$module / race"
      else label="$module / compile (no tests)"; fi
      run_suite "$label" env GOTOOLCHAIN=local GORACE="${GORACE:-atexit_sleep_ms=0}" \
        "${GO_BIN:-go}" -C "$module" test -race -count=1 ./...
      run_suite "$module / vet" env GOTOOLCHAIN=local "${GO_BIN:-go}" -C "$module" vet ./...
    done
  fi
fi
if [[ $group == all || $group == node ]]; then
  run_suite 'Node / Task Tab Groups + Pi extensions' node_suite
fi
printf '\nTest summary:\n'
printf '%s\n' "${summary[@]}"
printf 'Failed suites: %s\n' "$failures"
[[ $failures -eq 0 ]]
