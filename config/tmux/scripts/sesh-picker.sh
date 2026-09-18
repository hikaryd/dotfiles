#!/bin/bash
set -euo pipefail

# Popup может наследовать сокращённый PATH; полный brew shellenv здесь не нужен.
PATH="${PATH:-/usr/bin:/bin}"
case ":$PATH:" in *:/opt/homebrew/sbin:*) ;; *) PATH="/opt/homebrew/sbin:$PATH" ;; esac
case ":$PATH:" in *:/opt/homebrew/bin:*) ;; *) PATH="/opt/homebrew/bin:$PATH" ;; esac
export PATH

sel=$(sesh list -i \
  | gum filter --limit 1 --no-sort --fuzzy \
               --placeholder "Pick a sesh" --height 50 --prompt=" ")

[ -z "$sel" ] && exit 0

sel=$(printf "%s" "$sel" | tr -d "\r" | sed -E 's/[[:space:]]+$//')
exec sesh connect "$sel"
