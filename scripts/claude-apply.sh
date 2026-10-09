#!/usr/bin/env bash
# Apply Claude snapshots using the native runtime; source private shell secrets.
set -euo pipefail

CLAUDE_DIR="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$REPO_DIR/config/claude"
[[ -d "$SRC" ]] || { echo "claude-apply: source directory missing" >&2; exit 1; }
echo "claude-apply: применяю конфиг в $CLAUDE_DIR"

SECRETS="$CLAUDE_DIR/.mcp-secrets"
if [[ -f "$SECRETS" ]]; then
  set -a
  # shellcheck disable=SC1090
  . "$SECRETS"
  set +a
  echo "  секреты загружены из $SECRETS"
else
  echo "  секреты не найдены — MCP, требующие секретов, будут пропущены."
fi

HAVE_CLAUDE=0
command -v claude >/dev/null 2>&1 && HAVE_CLAUDE=1
export CLAUDE_DIR SRC HAVE_CLAUDE
# Validation precedes private backups and writes in the native implementation.
"$REPO_DIR/build/bin/dots-tools" setup claude-apply-json

if [[ -f "$HOME/.local/share/dots/abide/$(< "$REPO_DIR/config/abide/version")/node_modules/@coldtea/abide/package.json" ]]; then
  "$REPO_DIR/build/bin/dots-tools" setup abide-apply
fi
echo "claude-apply: готово"
