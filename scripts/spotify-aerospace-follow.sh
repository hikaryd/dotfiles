#!/usr/bin/env bash
# Keep Spotify's floating window on the currently focused AeroSpace workspace.
set -euo pipefail

pgrep -x Spotify >/dev/null || exit 0

target="${AEROSPACE_FOCUSED_WORKSPACE:-}"
if [[ -z "$target" ]]; then
  target="$(aerospace list-workspaces --focused 2>/dev/null || true)"
fi
[[ -n "$target" ]] || exit 0

aerospace list-windows --all --format '%{window-id}|%{app-bundle-id}|%{workspace}' |
  while IFS='|' read -r window_id app_id workspace; do
    if [[ "$app_id" == 'com.spotify.client' && "$workspace" != "$target" ]]; then
      aerospace move-node-to-workspace --window-id "$window_id" "$target"
    fi
  done
