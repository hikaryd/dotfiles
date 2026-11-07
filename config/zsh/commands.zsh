# Общие команды zsh: без prompt, completion, ZLE и фоновых hooks.
# Nushell вызывает этот модуль через отдельный zsh -f.
typeset -g DOTS_ROOT="${DOTS_ROOT:-${${(%):-%x}:A:h:h:h}}"

# --- Environment ---
export EDITOR="nvim"
export VISUAL="nvim"
export BUN_INSTALL="$HOME/.bun"

# Secrets, identities, service endpoints, and cluster coordinates stay outside
# this repository. See config/zsh/private.example.zsh for the supported keys.
typeset -g DOTS_PRIVATE_ZSH="${DOTS_PRIVATE_ZSH:-${XDG_CONFIG_HOME:-$HOME/.config}/dots/private.zsh}"
[[ -r "$DOTS_PRIVATE_ZSH" ]] && source "$DOTS_PRIVATE_ZSH"

# PATH / function and module paths (typeset -U removes duplicates)
typeset -U path fpath module_path
path=(
  "$BUN_INSTALL/bin"
  "$HOME/.cargo/bin"
  "$HOME/.local/bin"
  /opt/homebrew/bin
  /opt/homebrew/sbin
  /usr/local/bin
  ${path:#"$HOME/.dual-graph"}
  /Applications
)

# Completion/function search path must be ready before compinit.
# Стабильные (версионно-независимые) пути Homebrew добавлены ЯВНО: встроенный
# $fpath ниже указывает на Cellar/zsh/<версия>/…, который brew удаляет при
# `brew upgrade zsh` — тогда уже запущенные сессии ломаются с
# «_main_complete: function definition file not found». share-пути brew
# переносит на новую версию, поэтому они переживают апгрейд.
fpath=(
  "$HOME/.bun"
  "$DOTS_ROOT/config/zsh/functions"
  "$HOME/.config/zsh/functions"
  /opt/homebrew/share/zsh/site-functions
  /opt/homebrew/share/zsh/functions
  $fpath
)

# Homebrew embeds its current Cellar version in zsh's default module_path.
# Prefer the stable opt symlink so complist/computil keep loading after upgrades.
[[ -d /opt/homebrew/lib/zsh ]] && module_path=(/opt/homebrew/lib $module_path)

# --- Autoloaded functions (lazy — loaded only on first call) ---
autoload -Uz _dots-zoxide-init extract kafka-consume kafka-produce pytest-safe tp y z zi

# --- Aliases ---
alias v='nvim'
alias cat='bat --style=plain'
alias l='nls'
alias c='clear'
alias lg='lazygit'
alias jx5='env JCODE_RUNTIME_DIR="$HOME/.jcode/runtime-x5" jcode --socket "$HOME/.jcode/runtime-x5/jcode.sock" --provider-profile x5-copilot --model x5-airun-code-large'
alias gaa='git add -A'

alias ..='cd ..'
alias ...='cd ../..'
alias ....='cd ../../..'

alias bu='brew upgrade --cask --greedy'
alias speedtest='networkquality'
alias vs='source .venv/bin/activate'
alias share_port='npx tunnelmole 8000'
alias create_mr='"$DOTS_ROOT/scripts/ai_helper" --mode mr'
alias nvim-bench='hyperfine "nvim --startuptime /tmp/startup.log +qall" --warmup 3 --runs 10'

export FZF_DEFAULT_OPTS=" \
  --color=bg+:#232323,bg:#101010,spinner:#ffc799,hl:#ffc799 \
  --color=fg:#ffffff,header:#a0a0a0,info:#a0a0a0,pointer:#ffc799 \
  --color=marker:#99ffe4,fg+:#ffffff,prompt:#ffc799,hl+:#ffc799 \
  --color=selected-bg:#232323,border:#282828 \
  --border='rounded' --preview-window='border-rounded' \
  --prompt='> ' --marker='>' --pointer='›' --separator='─' --scrollbar='│'"

# Общий локальный VLESS-прокси (xray) для Jevgrep и Spotifast.
[[ -f "$DOTS_ROOT/config/shell/local-proxy.sh" ]] && source "$DOTS_ROOT/config/shell/local-proxy.sh"
