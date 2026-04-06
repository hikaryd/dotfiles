#!/usr/bin/env bash
# Session picker for Ghostty using gtab + gum
set -euo pipefail

eval "$(/opt/homebrew/bin/brew shellenv)"

GTAB_DIR="${GTAB_DIR:-$HOME/.config/gtab}"

if [[ ! -d "$GTAB_DIR" ]] || ! ls "$GTAB_DIR"/*.applescript &>/dev/null; then
  echo "No gtab workspaces found. Save one first: gtab save <name>"
  exit 0
fi

# List workspace names
workspaces=()
for f in "$GTAB_DIR"/*.applescript; do
  workspaces+=("$(basename "$f" .applescript)")
done

sel=$(printf '%s\n' "${workspaces[@]}" \
  | gum filter --limit 1 --no-sort --fuzzy \
               --placeholder "Pick a workspace" --height 20 --prompt=" ")

[[ -z "$sel" ]] && exit 0

exec gtab "$sel"
