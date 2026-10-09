#!/usr/bin/env bash
# Export Claude snapshots through native validation and secret masking.
set -euo pipefail

CLAUDE_DIR="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DST="$REPO_DIR/config/claude"
export CLAUDE_DIR DST HOME
"$REPO_DIR/build/bin/dots-tools" setup claude-sync-json

printf '\nГотово. Проверьте изменения перед коммитом:\n'
git -C "$REPO_DIR" status --short config/claude/
printf '\n  git -C "%s" add -p config/claude/\n' "$REPO_DIR"
