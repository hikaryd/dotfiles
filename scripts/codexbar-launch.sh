#!/usr/bin/env bash
# Launch CodexBar with Codex CLI quota reads through the existing local proxy.
set -euo pipefail
export PATH="/opt/homebrew/bin:/usr/local/bin:$HOME/.local/bin:$PATH"
REPO_DIR="$(cd "$(dirname "$(realpath "${BASH_SOURCE[0]}")")/.." && pwd)"

# shellcheck source=../config/shell/codex-proxy.sh
source "$HOME/.config/shell/codex-proxy.sh"
_codex_proxy_ensure
proxy="$CODEX_PROXY_URL"
codex_bin="$(command -v codex)"
# Merge only the Codex provider; never version or replace account credentials.
config_path="$("$REPO_DIR/build/bin/dots-tools" setup codexbar-config)"

# OAuth/WebView requests do not honor proxy env in CodexBar 0.60.0.
defaults write com.steipete.codexbar openAIWebAccessEnabled -bool false
# Per-process env only: never use launchctl setenv or system proxy settings.
open -a CodexBar \
  --env "CODEXBAR_CONFIG=$config_path" \
  --env "CODEX_CLI_PATH=$codex_bin" \
  --env "HTTP_PROXY=$proxy" --env "HTTPS_PROXY=$proxy" --env "ALL_PROXY=$proxy" \
  --env "http_proxy=$proxy" --env "https_proxy=$proxy" --env "all_proxy=$proxy" \
  --env "NO_PROXY=127.0.0.1,localhost,::1,*.local" \
  --env "no_proxy=127.0.0.1,localhost,::1,*.local"
