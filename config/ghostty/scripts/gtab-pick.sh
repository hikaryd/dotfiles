#!/usr/bin/env bash
# Native shared selector; original gtab workspace IDs remain unchanged.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
native="$root/build/bin/dots-tools"
export PATH="${PATH:-/usr/bin:/bin}:/opt/homebrew/bin:/opt/homebrew/sbin"
GTAB_DIR="${GTAB_DIR:-$HOME/.config/gtab}"
workspaces=()
shopt -s nullglob
for file in "$GTAB_DIR"/*.applescript; do
  workspaces+=("$(basename "$file" .applescript)")
done
if [[ ${#workspaces[@]} -eq 0 ]]; then
  echo "No gtab workspaces found. Save one first: gtab save <name>"
  exit 0
fi
if sel=$(printf '%s\n' "${workspaces[@]}" | "$native" ui select --title "Gtab · workspace" --mode LOCAL); then
  [[ -n "$sel" ]] && exec gtab "$sel"
else
  code=$?
  [[ $code -eq 130 ]] && exit 0
  exit "$code"
fi
