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

# Все Kubernetes-команды и guards живут в одном модуле. Его source не делает
# сетевых запросов; подключение к кластеру начинается только при вызове команды.
[[ -r "$DOTS_ROOT/config/zsh/kube-tools.zsh" ]] &&
  source "$DOTS_ROOT/config/zsh/kube-tools.zsh"

# PATH / function and module paths (typeset -U removes duplicates)
typeset -U path fpath module_path
path=(
  "$BUN_INSTALL/bin"
  "$HOME/.cargo/bin"
  "$HOME/.local/bin"
  /opt/homebrew/bin
  /opt/homebrew/sbin
  /usr/local/bin
  $path
  /Applications
  "$HOME/.dual-graph"
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
autoload -Uz _dots-zoxide-init extract graphify-merge-fix kafka-consume kafka-produce tp y z zi

# --- Aliases ---
alias v='nvim'
alias cat='bat --style=plain'
alias l='nls'
alias c='clear'
alias lg='lazygit'
alias gaa='git add -A'
alias gmf='graphify-merge-fix'

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
  --color=bg+:#273747,bg:#0d1017,spinner:#73d0ff,hl:#73d0ff \
  --color=fg:#bfbdb6,header:#858d9c,info:#858d9c,pointer:#73d0ff \
  --color=marker:#95e6cb,fg+:#bfbdb6,prompt:#73d0ff,hl+:#73d0ff \
  --color=selected-bg:#273747,border:#303847 \
  --border='rounded' --preview-window='border-rounded' \
  --prompt='> ' --marker='>' --pointer='›' --separator='─' --scrollbar='│'"

# Codex через изолированный VLESS-прокси (xray).
# Источник (общий для bash и zsh): ~/.config/shell/codex-proxy.sh
[[ -f "$DOTS_ROOT/config/shell/codex-proxy.sh" ]] && source "$DOTS_ROOT/config/shell/codex-proxy.sh"

