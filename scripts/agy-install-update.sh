#!/bin/bash
set -uo pipefail
status=0
bin="$HOME/.local/bin/agy"
if [[ -f "$bin" && ! -L "$bin" ]] && codesign --verify --strict \
    -R='anchor apple generic and certificate leaf[subject.OU] = "EQHXZ8M8AV" and identifier "cli"' "$bin" 2>/dev/null; then
  env -i HOME="$HOME" PATH="$PATH" "$bin" update </dev/null || status=1
else
  echo 'agy: not a verified Google CLI, skipping update' >&2
fi
export HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_ENV_HINTS=1 NONINTERACTIVE=1
python3 "$HOME/.config/dots/agy-install-manager-update.py" || status=1
exit "$status"
