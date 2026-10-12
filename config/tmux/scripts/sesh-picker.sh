#!/bin/bash
set -euo pipefail
# Preserve sesh connect semantics; dots sessions open may create fresh shells.
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
native="$root/build/bin/dots-tools"
export PATH="${PATH:-/usr/bin:/bin}:/opt/homebrew/bin:/opt/homebrew/sbin"
if sel=$(sesh list -i | "$native" ui select --title "Sesh · сессии" --mode LOCAL); then
  [[ -n "$sel" ]] && exec sesh connect "$sel"
else
  code=$?
  [[ $code -eq 130 ]] && exit 0
  exit "$code"
fi
