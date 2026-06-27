# =============================================================================
# Codex (oh-my-codex / omx) через изолированный VLESS-прокси (xray)
# -----------------------------------------------------------------------------
# Единый источник для bash и zsh. Симлинкуется в ~/.config/shell/codex-proxy.sh
# и подключается из ~/.zshrc и ~/.bashrc.
#
# Функция omx НЕ заменяет oh-my-codex, а оборачивает его:
#   1) поднимает локальный HTTP-прокси xray на 127.0.0.1:10810 (если не поднят);
#   2) запускает настоящий бинарник omx через `command omx "$@"`.
#
# Proxy-переменные задаются ТОЛЬКО процессу omx — системный трафик,
# корпоративный VPN и другие приложения не затрагиваются.
#
# Управление прокси: codex-proxy --status | --stop | --update | --list
# Настройка серверов:  codex-proxy --set-sub   (URL подписки)
# =============================================================================

alias codex-proxy='python3 "$HOME/.config/xray-codex/codex-proxy.py"'

omx() {
  local dir="$HOME/.config/xray-codex"
  local cfg="$dir/config.json"
  local pidf="$dir/xray.pid"
  local port="${CODEX_PROXY_PORT:-10810}"
  local proxy="http://127.0.0.1:$port"

  # Поднять xray, если локальный HTTP-прокси ещё не слушает порт.
  if ! lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    if [ ! -f "$cfg" ]; then
      echo "omx: нет $cfg — сначала настрой прокси: codex-proxy" >&2
      return 1
    fi
    local xray_bin
    xray_bin="$(command -v xray)" || {
      echo "omx: xray не найден (brew install xray)" >&2
      return 1
    }
    echo "omx: поднимаю VLESS-прокси на $proxy ..." >&2
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
      echo "omx: не удалось поднять xray на :$port (диагностика: codex-proxy --status)" >&2
      return 1
    fi
  fi

  # Запустить настоящий oh-my-codex (omx) с proxy-env только для этого процесса.
  HTTP_PROXY="$proxy" HTTPS_PROXY="$proxy" ALL_PROXY="$proxy" \
  http_proxy="$proxy" https_proxy="$proxy" all_proxy="$proxy" \
  NO_PROXY="127.0.0.1,localhost,::1,*.local" no_proxy="127.0.0.1,localhost,::1,*.local" \
  command omx "$@"
}
