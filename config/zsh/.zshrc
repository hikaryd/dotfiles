# =============================================================================
# ZSH Configuration — migrated from nushell, optimized for performance
# =============================================================================

# --- Environment ---
export EDITOR="nvim"
export VISUAL="nvim"
export OPENAI_BASE_URL="https://gateway.ai.cloudflare.com/v1/1a911fb4ac31b7d5e7b5a60fb08aa48f/aihr-proxy/openai"
export BUN_INSTALL="$HOME/.bun"

# Kubeconfig — default (stage/dev) loads automatically via native zsh glob (no fork)
if [[ -d "$HOME/.kube/configs/default" ]]; then
  local -a _kc=("$HOME/.kube/configs/default"/*(.N))
  (( ${#_kc} )) && export KUBECONFIG="${(j.:.)_kc}"
  unset _kc
fi

# kprod — load prod kubeconfigs with confirmation, then run kubectl (lazy: glob runs only on call)
kprod() {
  local -a prod_files=("$HOME/.kube/configs/prod"/*(.N))
  if (( ! ${#prod_files} )); then
    echo "No prod kubeconfigs found in ~/.kube/configs/prod/" >&2
    return 1
  fi
  if [[ -z "$KPROD_CONFIRMED" ]]; then
    printf "⚠️  PROD CLUSTER ACCESS — type 'yes' to proceed: "
    read -r answer
    [[ "$answer" = "yes" ]] || { echo "Aborted."; return 1; }
  fi
  KUBECONFIG="${(j.:.)prod_files}" kubectl "$@"
}

# PATH / fpath (typeset -U removes duplicates)
typeset -U path fpath
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
fpath=(
  "$HOME/.bun"
  "$HOME/.config/zsh/functions"
  $fpath
)

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
[[ -s "$_zcompdump" && (! -s "$_zcompdump.zwc" || "$_zcompdump" -nt "$_zcompdump.zwc") ]] && \
  zcompile "$_zcompdump" &!
unset _zcompdump _zcompdump_recent

zstyle ':completion:*' matcher-list 'm:{a-zA-Z}={A-Za-z}'
zstyle ':completion:*' menu select
zstyle ':completion:*' list-colors "${(s.:.)LS_COLORS}"
zstyle ':completion:*' use-cache on
zstyle ':completion:*' cache-path "$HOME/.zcompcache"

# --- Autoloaded functions (lazy — loaded only on first call) ---
autoload -Uz extract kafka-consume kafka-produce tp

# --- Key bindings ---
bindkey -e
bindkey '^A' beginning-of-line
bindkey '^E' end-of-line
bindkey '^P' autosuggest-accept
bindkey '^N' down-line-or-search

# --- Aliases ---
alias v='nvim'
alias cat='bat --style=plain'
alias l='ls'
alias c='clear'
alias lg='lazygit'
alias gaa='git add -A'

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
ZSH_AUTOSUGGEST_STRATEGY=(history completion)

# --- Integrations ---
# Starship: cache init script (avoids forking starship on every shell start)
_starship_cache="$HOME/.cache/starship-init.zsh"
if [[ ! -s "$_starship_cache" || "$(command -v starship)" -nt "$_starship_cache" ]]; then
  mkdir -p "$HOME/.cache"
  starship init zsh --print-full-init > "$_starship_cache" 2>/dev/null
fi
[[ -s "$_starship_cache" ]] && source "$_starship_cache"
unset _starship_cache

# fzf + plugins are loaded on first ZLE line-init instead of blocking shell
# startup. zsh-syntax-highlighting is still sourced last inside the loader.
_zsh_load_deferred_plugins() {
  (( ${+_zsh_deferred_plugins_loaded} )) && return
  typeset -g _zsh_deferred_plugins_loaded=1

  [[ -f /opt/homebrew/opt/fzf/shell/completion.zsh ]] && source /opt/homebrew/opt/fzf/shell/completion.zsh
  [[ -f /opt/homebrew/opt/fzf/shell/key-bindings.zsh ]] && source /opt/homebrew/opt/fzf/shell/key-bindings.zsh

  [[ -f /opt/homebrew/share/zsh-autosuggestions/zsh-autosuggestions.zsh ]] && \
    source /opt/homebrew/share/zsh-autosuggestions/zsh-autosuggestions.zsh
  [[ -f /opt/homebrew/share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh ]] && \
    source /opt/homebrew/share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh
}

if [[ -o interactive ]]; then
  autoload -Uz add-zle-hook-widget
  zle -N _zsh_load_deferred_plugins
  add-zle-hook-widget line-init _zsh_load_deferred_plugins
fi

# Codex (oh-my-codex / omx) через изолированный VLESS-прокси (xray).
# Источник (общий для bash и zsh): ~/.config/shell/codex-proxy.sh
[[ -f ~/.config/shell/codex-proxy.sh ]] && source ~/.config/shell/codex-proxy.sh
