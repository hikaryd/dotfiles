#!/usr/bin/env bash
# Install the official CLI; credentials stay in the user-owned Jevgrep config.
set -euo pipefail
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="$(< "$REPO_DIR/config/jevgrep/version")"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "jevgrep: invalid pinned version" >&2; exit 1; }
if ! command -v jg >/dev/null 2>&1 || [[ "$(command jg --version)" != "$VERSION" ]]; then
  npm install --global "@dzhng/jevgrep@$VERSION" --no-audit --no-fund
fi
"$REPO_DIR/build/bin/dots-tools" setup jevgrep-apply
