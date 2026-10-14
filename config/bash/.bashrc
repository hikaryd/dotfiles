# Ensure child processes see bash as SHELL
export SHELL=/bin/bash

# Pi через изолированный VLESS-прокси.
# Общий источник для bash и zsh: ~/.config/shell/pi-proxy.sh
[ -f "$HOME/.config/shell/pi-proxy.sh" ] && . "$HOME/.config/shell/pi-proxy.sh"
. "$HOME/.cargo/env"
