#!/usr/bin/env bash
set -euo pipefail
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="$HOME/.local/bin:$PATH"
"$REPO_DIR/build/bin/dots-tools" setup claude-statusbar-preflight
if [[ ! -x "$HOME/.local/bin/cs" ]]; then
  installer="$(mktemp)"
  trap 'rm -f "$installer"' EXIT
  curl -fsSL https://raw.githubusercontent.com/leeguooooo/claude-code-usage-bar/main/install.sh -o "$installer"
  bash "$installer" </dev/null
fi
CLAUDE_CONFIG_DIR="$HOME/.claude" "$HOME/.local/bin/cs" --setup
"$REPO_DIR/build/bin/dots-tools" setup claude-statusbar-apply
