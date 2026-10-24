#!/usr/bin/env bash
# Start Spotifast with the local HTTP proxy (Settings → Proxy → System).
set -euo pipefail

app="/Applications/Spotifast.app"
export PATH="/opt/homebrew/bin:/opt/homebrew/sbin:${PATH:-/usr/bin:/bin:/usr/sbin:/sbin}"
if [[ ! -d "$app" ]]; then
  echo "Spotifast is not installed (brew install --cask crmne/tap/spotifast)" >&2
  exit 1
fi

# shellcheck source=../config/shell/pi-proxy.sh
source "$HOME/.config/shell/pi-proxy.sh"
_pi_proxy_ensure spotifast
proxy="$PI_PROXY_URL"

if pgrep -x Spotifast >/dev/null; then
  open -a "$app"
  exit
fi

# System mode uses these variables for both Web API requests and local playback.
# Do not edit Spotifast's settings or change the system-wide proxy.
open -a "$app" \
  --env "HTTP_PROXY=$proxy" --env "HTTPS_PROXY=$proxy" --env "ALL_PROXY=$proxy" \
  --env "http_proxy=$proxy" --env "https_proxy=$proxy" --env "all_proxy=$proxy" \
  --env "NO_PROXY=127.0.0.1,localhost,::1,*.local" \
  --env "no_proxy=127.0.0.1,localhost,::1,*.local"
