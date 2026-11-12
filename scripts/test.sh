#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
# Load tmux.conf into a throwaway server; "run" lines are skipped so plugins
# (continuum restore) and theme hooks never touch real sessions.
conf=$(mktemp "${TMPDIR:-/tmp}/dots-tmux.XXXXXX")
trap 'tmux -L dots-check kill-server 2>/dev/null; rm -f "$conf"' EXIT
grep -v '^run' "${TMUX_CONF:-$root/config/tmux/tmux.conf}" > "$conf"
tmux -L dots-check -f /dev/null new-session -d 'sleep 60'
tmux -L dots-check source-file "$conf"
echo 'ok  	tmux.conf'
cd "$root/tools/dots"
export GOTOOLCHAIN=local
"${GO_BIN:-go}" vet ./...
"${GO_BIN:-go}" test -count=1 ./...
