#!/usr/bin/env bash
# Export the stable, non-secret subset of ~/.codex/config.toml into dotfiles.

set -euo pipefail

CODEX_DIR="${CODEX_CONFIG_DIR:-${CODEX_HOME:-$HOME/.codex}}"
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE="$CODEX_DIR/config.toml"
TARGET="$REPO_DIR/config/codex/config.toml"

if [[ ! -f $SOURCE ]]; then
  echo "codex-sync: config not found: $SOURCE" >&2
  exit 1
fi

mkdir -p "$(dirname "$TARGET")"

echo "codex-sync: exporting stable settings from $SOURCE"
python3 "$REPO_DIR/scripts/codex_config.py" sync \
  --source "$SOURCE" \
  --target "$TARGET"

echo
echo "Review changes before committing:"
git -C "$REPO_DIR" status --short config/codex/
echo
echo "  git -C '$REPO_DIR' add -p config/codex/"
