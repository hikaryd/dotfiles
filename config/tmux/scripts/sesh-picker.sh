#!/bin/bash
eval "$(/opt/homebrew/bin/brew shellenv)"
set -euo pipefail

sel=$(sesh list -i \
  | gum filter --limit 1 --no-sort --fuzzy \
               --placeholder "Pick a sesh" --height 50 --prompt=" ")

[ -z "$sel" ] && exit 0

sel=$(printf "%s" "$sel" | tr -d "\r" | sed -E 's/[[:space:]]+$//')
exec sesh connect "$sel"
