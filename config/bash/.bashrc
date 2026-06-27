# Ensure child processes see bash as SHELL
export SHELL=/bin/bash

# If interactive and not a Claude Code agent, switch to nushell
# if [[ $- == *i* ]] && [[ -z "$CLAUDECODE" ]]; then
# exec /opt/homebrew/bin/nu
# fi

# Codex (oh-my-codex / omx) через изолированный VLESS-прокси.
# Общий источник для bash и zsh: ~/.config/shell/codex-proxy.sh
[ -f "$HOME/.config/shell/codex-proxy.sh" ] && . "$HOME/.config/shell/codex-proxy.sh"
