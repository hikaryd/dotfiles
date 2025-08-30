#!/usr/bin/env bash
set -Eeuo pipefail
FILE="$HOME/.cache/yabai/prev_space"
[[ -f "$FILE" ]] || exit 0
exec "$HOME/.config/skhd/scripts/space-number.sh" "$(cat "$FILE")"
