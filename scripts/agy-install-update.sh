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
exit "$status"
