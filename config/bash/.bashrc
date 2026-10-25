# Ensure child processes see bash as SHELL
export SHELL=/bin/bash

# Общий локальный VLESS-прокси для Jevgrep и Spotifast.
[ -f "$HOME/.config/shell/local-proxy.sh" ] && . "$HOME/.config/shell/local-proxy.sh"
. "$HOME/.cargo/env"
