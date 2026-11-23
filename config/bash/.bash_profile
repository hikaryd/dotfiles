# Source .bashrc for consistency
if [[ -f ~/.bashrc ]]; then
  source ~/.bashrc
fi
. "$HOME/.cargo/env"
[ -r "$HOME/.config/shell/claude-guard.sh" ] && . "$HOME/.config/shell/claude-guard.sh"
[ -r "$HOME/.config/shell/agy-manager.sh" ] && . "$HOME/.config/shell/agy-manager.sh"


# Added by Antigravity CLI installer
export PATH="$HOME/.local/bin:$PATH"
