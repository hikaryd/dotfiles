#!/usr/bin/env bash
# Install fixed MCP executables before Codex configuration points to them.
set -euo pipefail

npm_root="$(npm root -g)"
ensure_package() {
  local package="$1" version="$2" binary="$3"
  if [[ -x "$binary" ]] && node -e \
    'process.exit(require(process.argv[1]).version === process.argv[2] ? 0 : 1)' \
    "$npm_root/$package/package.json" "$version" 2>/dev/null; then
    echo "$package@$version: installed"
    return
  fi
  npm install -g "$package@$version"
}
ensure_package '@railsblueprint/blueprint-mcp' '1.9.22' '/opt/homebrew/bin/blueprint-mcp'
ensure_package '@z_ai/mcp-server' '0.1.5' '/opt/homebrew/bin/zai-mcp-server'
