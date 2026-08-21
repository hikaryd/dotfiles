# =============================================================================
# ZSH Configuration — migrated from nushell, optimized for performance
# =============================================================================

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
[[ -r "$HOME/dots/config/zsh/kube-tools.zsh" ]] &&
  source "$HOME/dots/config/zsh/kube-tools.zsh"

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
  "$HOME/.config/zsh/functions"
  /opt/homebrew/share/zsh/site-functions
  /opt/homebrew/share/zsh/functions
  $fpath
)

# Homebrew embeds its current Cellar version in zsh's default module_path.
# Prefer the stable opt symlink so complist/computil keep loading after upgrades.
[[ -d /opt/homebrew/lib/zsh ]] && module_path=(/opt/homebrew/lib $module_path)

# --- History ---
HISTFILE="$HOME/.zsh_history"
HISTSIZE=100000
SAVEHIST=100000
setopt SHARE_HISTORY
setopt HIST_IGNORE_ALL_DUPS
setopt HIST_SAVE_NO_DUPS
setopt HIST_REDUCE_BLANKS
setopt HIST_VERIFY
setopt INC_APPEND_HISTORY

# --- Shell options ---
setopt AUTO_CD
setopt INTERACTIVE_COMMENTS
setopt NO_BEEP
setopt GLOB_DOTS

# --- Completion (daily cache; -C skips security check on warm runs) ---
autoload -Uz compinit
_zcompdump="$HOME/.zcompdump-${ZSH_VERSION}"
_zcompdump_recent=("${_zcompdump}"(N.mh-24))
if (( ${#_zcompdump_recent} )); then
  compinit -C -d "$_zcompdump"
else
  compinit -d "$_zcompdump"
fi

# Keep the completion dispatcher resident in long-lived tmux shells. Homebrew
# removes the previous Cellar directory during an upgrade; a lazy autoload from
# that deleted directory would otherwise fail the next time completion runs.
if ! autoload +X _main_complete 2>/dev/null; then
  rm -f "$_zcompdump" "$_zcompdump.zwc"
  compinit -d "$_zcompdump"
  autoload +X _main_complete
fi

[[ -s "$_zcompdump" && (! -s "$_zcompdump.zwc" || "$_zcompdump" -nt "$_zcompdump.zwc") ]] && \
  zcompile "$_zcompdump" &!
unset _zcompdump _zcompdump_recent

zstyle ':completion:*' matcher-list 'm:{a-zA-Z}={A-Za-z}'
zstyle ':completion:*' menu select
zstyle ':completion:*' list-colors "${(s.:.)LS_COLORS}"
zstyle ':completion:*' use-cache on
zstyle ':completion:*' cache-path "$HOME/.zcompcache"

# --- Autoloaded functions (lazy — loaded only on first call) ---
autoload -Uz _dots-zoxide-init extract graphify-merge-fix kafka-consume kafka-produce tp y z zi

# --- Key bindings ---
bindkey -e
bindkey '^A' beginning-of-line
bindkey '^E' end-of-line
bindkey '^P' autosuggest-accept
bindkey '^N' down-line-or-search
autoload -Uz edit-command-line
zle -N edit-command-line
bindkey '^[e' edit-command-line

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
alias deploy-dev='~/dots/scripts/deploy-dev.sh'
alias speedtest='networkquality'
alias codex='codex -a untrusted -c model_reasoning_effort="high"'
alias vs='source .venv/bin/activate'
alias share_port='npx tunnelmole 8000'
alias create_mr='~/dots/scripts/ai_helper --mode mr'
alias nvim-bench='hyperfine "nvim --startuptime /tmp/startup.log +qall" --warmup 3 --runs 10'

# --- Catppuccin Mocha — FZF ---
export FZF_DEFAULT_OPTS=" \
  --color=bg+:#313244,bg:#1e1e2e,spinner:#f5e0dc,hl:#f38ba8 \
  --color=fg:#cdd6f4,header:#f38ba8,info:#cba6f7,pointer:#f5e0dc \
  --color=marker:#b4befe,fg+:#cdd6f4,prompt:#cba6f7,hl+:#f38ba8 \
  --color=selected-bg:#45475a \
  --border='rounded' --preview-window='border-rounded' \
  --prompt='> ' --marker='>' --pointer='◆' --separator='─' --scrollbar='│'"

# --- Catppuccin Mocha — syntax highlighting ---
typeset -A ZSH_HIGHLIGHT_STYLES
ZSH_HIGHLIGHT_MAXLENGTH=512
ZSH_HIGHLIGHT_STYLES[command]='fg=#89b4fa'
ZSH_HIGHLIGHT_STYLES[builtin]='fg=#89b4fa'
ZSH_HIGHLIGHT_STYLES[alias]='fg=#89b4fa'
ZSH_HIGHLIGHT_STYLES[function]='fg=#89b4fa'
ZSH_HIGHLIGHT_STYLES[unknown-token]='fg=#f38ba8'
ZSH_HIGHLIGHT_STYLES[path]='fg=#f9e2af,underline'
ZSH_HIGHLIGHT_STYLES[single-quoted-argument]='fg=#a6e3a1'
ZSH_HIGHLIGHT_STYLES[double-quoted-argument]='fg=#a6e3a1'
ZSH_HIGHLIGHT_STYLES[dollar-quoted-argument]='fg=#a6e3a1'
ZSH_HIGHLIGHT_STYLES[commandseparator]='fg=#89dceb'
ZSH_HIGHLIGHT_STYLES[redirection]='fg=#89dceb'
ZSH_HIGHLIGHT_STYLES[globbing]='fg=#f9e2af'
ZSH_HIGHLIGHT_STYLES[assign]='fg=#f2cdcd'
ZSH_HIGHLIGHT_STYLES[reserved-word]='fg=#cba6f7'
ZSH_HIGHLIGHT_STYLES[comment]='fg=#6c7086'
ZSH_HIGHLIGHT_STYLES[arg0]='fg=#89b4fa'

# --- Catppuccin Mocha — autosuggestions ---
ZSH_AUTOSUGGEST_HIGHLIGHT_STYLE='fg=#585b70'
# Completion-based suggestions spawn a nested completion PTY for novel input.
# History-only suggestions stay asynchronous and avoid input/Ctrl+C stalls.
ZSH_AUTOSUGGEST_STRATEGY=(history)
ZSH_AUTOSUGGEST_BUFFER_MAX_SIZE=256
typeset -g ZSH_AUTOSUGGEST_MANUAL_REBIND=1

# --- Integrations ---
# Native prompt avoids a Starship process on every redraw. Set
# DOTS_USE_STARSHIP=1 before starting zsh to restore the full Starship prompt.
if [[ "${DOTS_USE_STARSHIP:-0}" == 1 ]]; then
  _starship_cache="$HOME/.cache/starship-init.zsh"
  if [[ ! -s "$_starship_cache" || "$(command -v starship)" -nt "$_starship_cache" ]]; then
    mkdir -p "$HOME/.cache"
    starship init zsh --print-full-init > "$_starship_cache" 2>/dev/null
  fi
  [[ -s "$_starship_cache" ]] && source "$_starship_cache"
  unset _starship_cache
else
  setopt PROMPT_SUBST
  zmodload zsh/datetime
  autoload -Uz add-zsh-hook _dots-prompt-find-git-dir _dots-prompt-precmd _dots-prompt-preexec
  add-zsh-hook preexec _dots-prompt-preexec
  add-zsh-hook precmd _dots-prompt-precmd
  PROMPT='%F{#bfbdb6}󰀵 %5~ %f'$'\n> '
fi

# These integrations must exist before ZLE draws its first editable line.
# Registering a line-init hook from inside .zshrc is one prompt too late: the
# hook first runs after the user submits a command, so the initial prompt has
# neither history suggestions nor fzf completion.
if [[ -o interactive ]]; then
  [[ -f /opt/homebrew/opt/fzf/shell/completion.zsh ]] && source /opt/homebrew/opt/fzf/shell/completion.zsh
  [[ -f /opt/homebrew/opt/fzf/shell/key-bindings.zsh ]] && source /opt/homebrew/opt/fzf/shell/key-bindings.zsh

  if [[ -f /opt/homebrew/opt/fzf-tab/share/fzf-tab/fzf-tab.zsh ]]; then
    zstyle ':completion:*' menu no
    zstyle ':completion:*:descriptions' format '[%d]'
    zstyle ':completion:*:git-checkout:*' sort false
    zstyle ':fzf-tab:*' fzf-flags --height=60% \
      --bind=ctrl-j:down,ctrl-k:up,ctrl-c:abort,tab:accept
    zstyle ':fzf-tab:*' switch-group '<' '>'
    zstyle ':fzf-tab:complete:*:*' fzf-preview \
      'if [ -d "$realpath" ]; then /bin/ls -laG -- "$realpath"; elif [ -f "$realpath" ]; then bat --color=always --style=numbers --line-range=:200 -- "$realpath"; fi'
    source /opt/homebrew/opt/fzf-tab/share/fzf-tab/fzf-tab.zsh
  fi

  [[ -f /opt/homebrew/share/zsh-autosuggestions/zsh-autosuggestions.zsh ]] && \
    source /opt/homebrew/share/zsh-autosuggestions/zsh-autosuggestions.zsh
  # zsh-syntax-highlighting must remain the last ZLE plugin sourced.
  [[ -f /opt/homebrew/share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh ]] && \
    source /opt/homebrew/share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh
fi

# Codex (oh-my-codex / omx) через изолированный VLESS-прокси (xray).
# Источник (общий для bash и zsh): ~/.config/shell/codex-proxy.sh
[[ -f ~/.config/shell/codex-proxy.sh ]] && source ~/.config/shell/codex-proxy.sh

# Run OMX without the tmux HUD/status pane.
export OMX_LAUNCH_POLICY=direct
# Native hooks already deliver notifications. The fallback watcher polls large
# OMX state trees and can consume a full CPU core per concurrent session.
export OMX_NOTIFY_FALLBACK=0
