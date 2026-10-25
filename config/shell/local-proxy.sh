# shellcheck shell=bash
# Shared bash/zsh module for Jevgrep and Spotifast through the local VLESS proxy.
# Proxy settings apply to child processes only, never to the parent shell.
alias dots-proxy='dots-tools proxy local'

_dots_proxy_ensure() {
  local client="${1:-client}"
  # Check local readiness without consuming the caller's stdin or changing routes.
  if ! command dots-tools proxy local --ensure </dev/null >&2; then
    echo "$client: прокси не готов (dots-proxy --install-service; dots-proxy --status)" >&2
    return 1
  fi
  DOTS_PROXY_URL="http://127.0.0.1:${DOTS_PROXY_PORT:-${PI_PROXY_PORT:-10810}}"
}

unalias jg 2>/dev/null || true
jg() {
  if [ "$#" -eq 1 ] && [ "$1" = doctor ]; then
    # Status 3 means a different provider, so use the official doctor below.
    local result=0
    command dots-tools proxy beatapi doctor --if-connected </dev/null || result=$?
    [ "$result" -eq 3 ] || return "$result"
  fi
  _dots_proxy_ensure jg || return
  local proxy="$DOTS_PROXY_URL"
  HTTP_PROXY="$proxy" HTTPS_PROXY="$proxy" ALL_PROXY="$proxy" \
  http_proxy="$proxy" https_proxy="$proxy" all_proxy="$proxy" \
  NO_PROXY="127.0.0.1,localhost,::1,*.local" no_proxy="127.0.0.1,localhost,::1,*.local" \
  NODE_USE_ENV_PROXY=1 command jg "$@"
}
