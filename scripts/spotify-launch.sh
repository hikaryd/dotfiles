#!/usr/bin/env bash
# Start the official Spotify app with its persistent local HTTP proxy.
set -euo pipefail

app="/Applications/Spotify.app"
script_dir="$(dirname "$(realpath "${BASH_SOURCE[0]}")")"
export PATH="/opt/homebrew/bin:/opt/homebrew/sbin:${PATH:-/usr/bin:/bin:/usr/sbin:/sbin}"
if [[ ! -d "$app" ]]; then
  echo "Spotify is not installed (brew install --cask spotify)" >&2
  exit 1
fi

# shellcheck source=../config/shell/codex-proxy.sh
source "$HOME/.config/shell/codex-proxy.sh"
_codex_proxy_ensure
proxy="$CODEX_PROXY_URL"
port="${CODEX_PROXY_PORT:-10810}"

config_status=0
"$script_dir/../build/bin/dots-tools" proxy spotify --port "$port" --check || config_status=$?
if [[ "$config_status" != 0 && "$config_status" != 1 ]]; then
  exit "$config_status"
fi
if [[ "$config_status" == 1 ]]; then
  running_pids="$(pgrep -x Spotify || true)"
  if [[ -n "$running_pids" ]]; then
    while IFS= read -r pid; do
      if [[ "$(ps -p "$pid" -o comm=)" != "$app/Contents/MacOS/Spotify" ]]; then
        echo "Unexpected process named Spotify ($pid); refusing to restart it" >&2
        exit 1
      fi
    done <<< "$running_pids"
    while IFS= read -r pid; do
      kill -TERM "$pid"
    done <<< "$running_pids"
    for _ in {1..50}; do
      pgrep -x Spotify >/dev/null || break
      sleep 0.1
    done
    if pgrep -x Spotify >/dev/null; then
      echo "Spotify did not exit after SIGTERM; proxy settings were not changed" >&2
      exit 1
    fi
  fi
  "$script_dir/../build/bin/dots-tools" proxy spotify --port "$port"
fi

if pgrep -x Spotify >/dev/null; then
  open -a "$app"
  exit
fi

open -a "$app" \
  --env "HTTP_PROXY=$proxy" --env "HTTPS_PROXY=$proxy" --env "ALL_PROXY=$proxy" \
  --env "http_proxy=$proxy" --env "https_proxy=$proxy" --env "all_proxy=$proxy" \
  --env "NO_PROXY=127.0.0.1,localhost,::1,*.local" \
  --env "no_proxy=127.0.0.1,localhost,::1,*.local"
