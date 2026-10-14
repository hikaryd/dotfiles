# shellcheck shell=bash
# CLI-инструменты через локальный VLESS-прокси. Общий модуль bash/zsh.
# Прокси задаётся только дочерним процессам; системные настройки не меняются.
# Управление: pi-proxy --status | --stop | --update | --list

alias pi-proxy='dots-tools proxy pi'

_pi_proxy_ensure() {
  local client="${1:-pi}"
  local dir="$HOME/.config/xray-pi"
  local cfg="$dir/config.json"
  local pidf="$dir/xray.pid"
  local port="${PI_PROXY_PORT:-10810}"
  local proxy="http://127.0.0.1:$port"

  # Поднять xray, если локальный HTTP-прокси ещё не слушает порт.
  if ! lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    if [ ! -f "$cfg" ]; then
      echo "$client: нет $cfg — сначала настрой прокси: pi-proxy" >&2
      return 1
    fi
    local xray_bin
    xray_bin="$(command -v xray)" || {
      echo "$client: xray не найден (brew install xray)" >&2
      return 1
    }
    echo "$client: поднимаю VLESS-прокси на $proxy ..." >&2
    nohup "$xray_bin" run -c "$cfg" >/dev/null 2>&1 &
    printf '%s\n' "$!" > "$pidf"
    disown 2>/dev/null || true
    local i=0
    while [ "$i" -lt 20 ]; do
      lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1 && break
      sleep 0.2
      i=$((i + 1))
    done
    if ! lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
      echo "$client: не удалось поднять xray на :$port (диагностика: pi-proxy --status)" >&2
      return 1
    fi
  fi

  PI_PROXY_URL="$proxy"
}

# Pi использует proxy env для HTTP и WebSocket; xray поднимается до запуска CLI.
unalias pi 2>/dev/null || true
pi() {
  _pi_proxy_ensure pi || return
  local proxy="$PI_PROXY_URL"
  HTTP_PROXY="$proxy" HTTPS_PROXY="$proxy" ALL_PROXY="$proxy" \
  http_proxy="$proxy" https_proxy="$proxy" all_proxy="$proxy" \
  WS_PROXY="$proxy" WSS_PROXY="$proxy" ws_proxy="$proxy" wss_proxy="$proxy" \
  NO_PROXY="127.0.0.1,localhost,::1,*.local,nebula-staging.x5.ru" no_proxy="127.0.0.1,localhost,::1,*.local,nebula-staging.x5.ru" \
  command pi "$@"
}

# Jevgrep (jg) использует Node fetch: proxy env задаётся только этому процессу.
unalias jg 2>/dev/null || true
jg() {
  _pi_proxy_ensure jg || return
  local proxy="$PI_PROXY_URL"
  HTTP_PROXY="$proxy" HTTPS_PROXY="$proxy" ALL_PROXY="$proxy" \
  http_proxy="$proxy" https_proxy="$proxy" all_proxy="$proxy" \
  NO_PROXY="127.0.0.1,localhost,::1,*.local" no_proxy="127.0.0.1,localhost,::1,*.local" \
  NODE_USE_ENV_PROXY=1 command jg "$@"
}
