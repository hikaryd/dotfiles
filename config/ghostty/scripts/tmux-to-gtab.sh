#!/usr/bin/env bash
# Migrate tmux sessions to gtab workspaces
set -euo pipefail

GTAB_DIR="${GTAB_DIR:-$HOME/.config/gtab}"
mkdir -p "$GTAB_DIR"

if ! tmux list-sessions &>/dev/null; then
  echo "tmux server not running — start tmux first"
  exit 1
fi

count=0

tmux list-sessions -F '#{session_name}' | while read -r session; do
  safe_name="${session//\//-}"
  file="$GTAB_DIR/$safe_name.applescript"

  if [[ -f "$file" ]]; then
    echo "skip: $session → $safe_name (already exists)"
    continue
  fi

  out='tell application "Ghostty"\n    activate'
  i=0

  while IFS=$'\t' read -r idx name wd; do
    i=$((i + 1))
    [[ -z "$wd" ]] && wd="$HOME"

    out+="\n\n    set cfg$i to new surface configuration"
    out+="\n    set initial working directory of cfg$i to \"$wd\""

    if [[ "$i" -eq 1 ]]; then
      out+="\n    set win to new window with configuration cfg$i"
      out+="\n    set term$i to focused terminal of selected tab of win"
    else
      out+="\n    set tab$i to new tab in win with configuration cfg$i"
      out+="\n    set term$i to focused terminal of tab$i"
    fi

    # Use window name as tab title (skip generic names)
    if [[ -n "$name" && "$name" != "bash" && "$name" != "zsh" && "$name" != "nu" ]]; then
      out+="\n    perform action \"set_tab_title:$name\" on term$i"
    fi
  done < <(tmux list-windows -t "$session" -F '#{window_index}	#{window_name}	#{pane_current_path}')

  out+="\nend tell"

  printf '%b' "$out" > "$file"
  echo "  ok: $session → $safe_name ($i tabs)"
done

echo ""
echo "Done. Use 'gtab list' to see workspaces, 'gtab <name>' to launch."
