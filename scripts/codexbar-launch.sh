#!/usr/bin/env bash
# Launch CodexBar with Codex CLI quota reads through the existing local proxy.
set -euo pipefail

# shellcheck source=../config/shell/codex-proxy.sh
source "$HOME/.config/shell/codex-proxy.sh"
_codex_proxy_ensure
proxy="$CODEX_PROXY_URL"
codex_bin="$(command -v codex)"

# Merge only the Codex provider; never version or replace account credentials.
config_path="$(python3 - <<'PY'
import json
import os
from pathlib import Path
import tempfile

current = Path.home() / ".config/codexbar/config.json"
legacy = Path.home() / ".codexbar/config.json"
path = current if current.exists() or not legacy.exists() else legacy
xdg = os.environ.get("XDG_CONFIG_HOME", "")
if xdg and Path(xdg).is_absolute():
    path = Path(xdg) / "codexbar/config.json"
override = os.environ.get("CODEXBAR_CONFIG", "").strip()
if override:
    path = Path(override).expanduser()
path = path.absolute()
data = json.loads(path.read_text()) if path.exists() else {"version": 1, "providers": []}
providers = data.setdefault("providers", [])
codex = next((provider for provider in providers if provider.get("id") == "codex"), None)
if codex is None:
    codex = {"id": "codex"}
    providers.append(codex)
codex.update(enabled=True, source="cli")
path.parent.mkdir(parents=True, exist_ok=True)
fd, temporary = tempfile.mkstemp(prefix=".config-", dir=path.parent)
try:
    with os.fdopen(fd, "w") as file:
        json.dump(data, file, indent=2)
        file.write("\n")
    os.replace(temporary, path)
finally:
    if os.path.exists(temporary):
        os.unlink(temporary)
print(path)
PY
)"

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
