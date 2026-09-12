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
alias speedtest='networkquality'
alias codex='codex -a untrusted -c model_reasoning_effort="high"'
alias vs='source .venv/bin/activate'
alias share_port='npx tunnelmole 8000'
alias create_mr='~/dots/scripts/ai_helper --mode mr'
alias nvim-bench='hyperfine "nvim --startuptime /tmp/startup.log +qall" --warmup 3 --runs 10'

# --- Quiet Ayu — поиск и подсветка ввода ---
export FZF_DEFAULT_OPTS=" \
  --color=bg+:#273747,bg:#0d1017,spinner:#73d0ff,hl:#73d0ff \
  --color=fg:#bfbdb6,header:#858d9c,info:#858d9c,pointer:#73d0ff \
  --color=marker:#95e6cb,fg+:#bfbdb6,prompt:#73d0ff,hl+:#73d0ff \
  --color=selected-bg:#273747,border:#303847 \
  --border='rounded' --preview-window='border-rounded' \
  --prompt='> ' --marker='>' --pointer='›' --separator='─' --scrollbar='│'"

typeset -A ZSH_HIGHLIGHT_STYLES
ZSH_HIGHLIGHT_MAXLENGTH=512
ZSH_HIGHLIGHT_STYLES[command]='fg=#73d0ff'
ZSH_HIGHLIGHT_STYLES[builtin]='fg=#73d0ff'
ZSH_HIGHLIGHT_STYLES[alias]='fg=#73d0ff'
ZSH_HIGHLIGHT_STYLES[function]='fg=#73d0ff'
ZSH_HIGHLIGHT_STYLES[unknown-token]='fg=#f07178'
ZSH_HIGHLIGHT_STYLES[path]='fg=#bfbdb6,underline'
ZSH_HIGHLIGHT_STYLES[single-quoted-argument]='fg=#95e6cb'
ZSH_HIGHLIGHT_STYLES[double-quoted-argument]='fg=#95e6cb'
ZSH_HIGHLIGHT_STYLES[dollar-quoted-argument]='fg=#95e6cb'
ZSH_HIGHLIGHT_STYLES[commandseparator]='fg=#858d9c'
ZSH_HIGHLIGHT_STYLES[redirection]='fg=#858d9c'
ZSH_HIGHLIGHT_STYLES[globbing]='fg=#e6b450'
ZSH_HIGHLIGHT_STYLES[assign]='fg=#bfbdb6'
ZSH_HIGHLIGHT_STYLES[reserved-word]='fg=#73d0ff'
ZSH_HIGHLIGHT_STYLES[comment]='fg=#858d9c'
ZSH_HIGHLIGHT_STYLES[arg0]='fg=#73d0ff'

# Подсказка слабее ввода, но остаётся читаемой.
ZSH_AUTOSUGGEST_HIGHLIGHT_STYLE='fg=#858d9c'
# Completion-based suggestions spawn a nested completion PTY for novel input.
# History-only suggestions stay asynchronous and avoid input/Ctrl+C stalls.
ZSH_AUTOSUGGEST_STRATEGY=(history)
ZSH_AUTOSUGGEST_BUFFER_MAX_SIZE=256
typeset -g ZSH_AUTOSUGGEST_MANUAL_REBIND=1

# --- Integrations ---
# Уведомления независимы от prompt provider, не сохраняют текст команд.
if [[ -o interactive ]]; then
  zmodload zsh/datetime
  autoload -Uz add-zsh-hook _dots-notify-preexec _dots-notify-precmd
  add-zsh-hook preexec _dots-notify-preexec
  add-zsh-hook precmd _dots-notify-precmd
fi

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
  PROMPT='%F{#858d9c}󰀵 %f%F{#bfbdb6}%5~ %f'$'\n''%F{#73d0ff}>%f '
fi

# These integrations must exist before ZLE draws its first editable line.
# Registering a line-init hook from inside .zshrc is one prompt too late: the
# hook first runs after the user submits a command, so the initial prompt has
# neither history suggestions nor fzf completion.
# Non-TTY `zsh -ic` has no editable line: fzf's option restore otherwise emits
# "can't change option: zle". Реальные интерактивные панели сохраняют все plugins.
if [[ -o interactive && -t 0 && -t 1 ]]; then
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
  # Только Ctrl-R; Up, fzf Alt-C/Ctrl-T и быстрые history autosuggestions прежние.
  # Cache исполняется из private user dir; invalidation по binary/config версии.
  if [[ -t 0 && -t 1 ]] && (( $+commands[atuin] )); then
    _atuin_cache_dir="${XDG_CACHE_HOME:-$HOME/.cache}/dots"
    _atuin_cache="$_atuin_cache_dir/atuin-init.zsh"
    if [[ ! -s "$_atuin_cache" || "$commands[atuin]" -nt "$_atuin_cache" || "$HOME/.zshrc" -nt "$_atuin_cache" ]]; then
      (umask 077; mkdir -p "$_atuin_cache_dir"; atuin init zsh --disable-up-arrow --disable-ai > "$_atuin_cache.$$.tmp" && mv -f "$_atuin_cache.$$.tmp" "$_atuin_cache")
    fi
    [[ -r "$_atuin_cache" ]] && source "$_atuin_cache"
    ZSH_AUTOSUGGEST_STRATEGY=(history)
    unset _atuin_cache _atuin_cache_dir
  fi

  _dots-menu-widget() {
    zle -I
    command dots menu
    zle reset-prompt
  }
  zle -N _dots-menu-widget
  bindkey '^O' _dots-menu-widget

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
