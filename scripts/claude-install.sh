#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
[[ $# -eq 0 ]] || { printf 'Usage: scripts/claude-install.sh\n' >&2; exit 2; }
exec /usr/bin/python3 -I "$root/config/claude/install_local.py"
