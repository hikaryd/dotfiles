# =============================================================================
# ZSH Configuration — migrated from nushell, optimized for performance
# =============================================================================

# --- Environment ---
export EDITOR="nvim"
export VISUAL="nvim"

# PATH (typeset -U removes duplicates)
typeset -U path
path=(
  "$HOME/.cargo/bin"
  "$HOME/.local/bin"
  /opt/homebrew/bin
  /usr/local/bin
  $path
  /Applications
  "$HOME/.dual-graph"
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

# --- Completion (with daily cache) ---
autoload -Uz compinit
if [[ -n "$HOME/.zcompdump"(#qN.mh+24) ]]; then
  compinit
else
  compinit -C
fi

zstyle ':completion:*' matcher-list 'm:{a-zA-Z}={A-Za-z}'
zstyle ':completion:*' menu select
zstyle ':completion:*' list-colors "${(s.:.)LS_COLORS}"
zstyle ':completion:*' use-cache on
zstyle ':completion:*' cache-path "$HOME/.zcompcache"

# --- Autoloaded functions (lazy — loaded only on first call) ---
fpath=("$HOME/.config/zsh/functions" $fpath)
autoload -Uz extract kafka-consume kafka-produce

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

alias fg='froggit'
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
eval "$(starship init zsh)"
eval "$(zoxide init zsh)"

# fzf
[[ -f /opt/homebrew/opt/fzf/shell/completion.zsh ]] && source /opt/homebrew/opt/fzf/shell/completion.zsh
[[ -f /opt/homebrew/opt/fzf/shell/key-bindings.zsh ]] && source /opt/homebrew/opt/fzf/shell/key-bindings.zsh

# --- Plugins (must be last) ---
[[ -f /opt/homebrew/share/zsh-autosuggestions/zsh-autosuggestions.zsh ]] && \
  source /opt/homebrew/share/zsh-autosuggestions/zsh-autosuggestions.zsh
[[ -f /opt/homebrew/share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh ]] && \
  source /opt/homebrew/share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh
