#!/usr/bin/env bash
# Hide or restore Spotify without depending on which app has keyboard focus.
set -euo pipefail

launcher="$HOME/.local/bin/spotify-app"
script_dir="$(dirname "$(realpath "${BASH_SOURCE[0]}")")"
cache_dir="$HOME/.cache/dots"
binary="$cache_dir/spotify-toggle"
source="$script_dir/spotify-toggle.swift"

mkdir -p "$cache_dir"
if [[ "${1:-}" != '--locked' ]]; then
  exec /usr/bin/lockf -t 15 "$cache_dir/spotify-toggle.lock" "$script_dir/spotify-toggle.sh" --locked
fi

if [[ ! -x "$binary" || "$source" -nt "$binary" ]]; then
  temporary="$(mktemp "$binary.XXXXXX")"
  trap 'rm -f "$temporary"' EXIT
  /usr/bin/swiftc -O "$source" -o "$temporary"
  chmod +x "$temporary"
  mv -f "$temporary" "$binary"
  trap - EXIT
fi

action="$("$binary")"
if [[ "$action" == 'launch' || "$action" == 'shown' ]]; then
  exec "$launcher"
fi
