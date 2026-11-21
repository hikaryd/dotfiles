#!/bin/bash
set -euo pipefail

bin="$HOME/.local/bin"
if [[ ! -e "$bin/agy" && ! -L "$bin/agy" ]]; then
  mkdir -p "$bin"
  staging=$(mktemp -d "$bin/.agy-install.XXXXXXXX")
  trap 'rm -rf "$staging"' EXIT
  mkdir "$staging/home" "$staging/bin"
  curl --proto '=https' --tlsv1.2 -fsSL https://antigravity.google/cli/install.sh -o "$staging/install.sh"
  HOME="$staging/home" XDG_CONFIG_HOME="$staging/home/.config" XDG_DATA_HOME="$staging/home/.local/share" \
    /bin/bash "$staging/install.sh" --dir "$staging/bin"
  [[ -x "$staging/bin/agy" ]] || { echo 'agy: installer produced no executable' >&2; exit 1; }
  # Hard-link creation fails rather than replacing a binary installed concurrently.
  ln "$staging/bin/agy" "$bin/agy"
fi
