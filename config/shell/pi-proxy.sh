# shellcheck shell=bash
# Общий модуль bash/zsh: Pi, jg и Spotifast через VLESS-прокси вне РФ.
# Прокси задаётся только дочерним процессам; системные настройки не меняются.
# Фоновая служба: pi-proxy --install-service | --status | --uninstall-service | --stop

alias pi-proxy='dots-tools proxy pi'

_pi_proxy_ensure() {
  local client="${1:-pi}"
  # Только локальная готовность фоновой службы: без сети, запуска и смены xray.
  # Не трогаем stdin: там может быть prompt для pi -p или входные данные jg.
  if ! command dots-tools proxy pi --ensure </dev/null >&2; then
    echo "$client: прокси не готов (pi-proxy --install-service; pi-proxy --status)" >&2
    return 1
  fi
  PI_PROXY_URL="http://127.0.0.1:${PI_PROXY_PORT:-10810}"
}

# Proxy env только у Pi; shell-команды агента очищают его через shellCommandPrefix.
unalias pi 2>/dev/null || true
pi() (
  _pi_proxy_ensure pi || return
  local proxy="$PI_PROXY_URL"
  export HTTP_PROXY="$proxy" HTTPS_PROXY="$proxy" ALL_PROXY="$proxy" \
  http_proxy="$proxy" https_proxy="$proxy" all_proxy="$proxy" \
  WS_PROXY="$proxy" WSS_PROXY="$proxy" ws_proxy="$proxy" wss_proxy="$proxy" \
  NO_PROXY="127.0.0.1,localhost,::1,*.local,private.invalid" no_proxy="127.0.0.1,localhost,::1,*.local,private.invalid"
  # Отдельный процесс: не ждём обновления и не отдаём ему терминал или prompt.
  (nohup pi update --extensions </dev/null >/dev/null 2>&1 &)
  command pi "$@"
)

# Jevgrep (jg) использует Node fetch: proxy env задаётся только этому процессу.
unalias jg 2>/dev/null || true
jg() {
  if [ "$#" -eq 1 ] && [ "$1" = doctor ]; then
    # Локальный BeatAPI: одна проверка, без многочасовых retry в jg 0.8.
    # Код 3 означает другой provider: сохраняем официальный doctor.
    local result=0
    command dots-tools proxy beatapi doctor --if-connected </dev/null || result=$?
    [ "$result" -eq 3 ] || return "$result"
  fi
  _pi_proxy_ensure jg || return
  local proxy="$PI_PROXY_URL"
  HTTP_PROXY="$proxy" HTTPS_PROXY="$proxy" ALL_PROXY="$proxy" \
  http_proxy="$proxy" https_proxy="$proxy" all_proxy="$proxy" \
  NO_PROXY="127.0.0.1,localhost,::1,*.local" no_proxy="127.0.0.1,localhost,::1,*.local" \
  NODE_USE_ENV_PROXY=1 command jg "$@"
}
