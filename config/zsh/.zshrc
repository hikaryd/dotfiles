# =============================================================================
# ZSH Configuration — migrated from nushell, optimized for performance
# =============================================================================

source "${DOTS_ROOT:-${${(%):-%x}:A:h:h:h}}/config/zsh/commands.zsh"

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

# --- Key bindings ---
bindkey -e
bindkey '^A' beginning-of-line
bindkey '^E' end-of-line
bindkey '^P' autosuggest-accept
bindkey '^N' down-line-or-search
autoload -Uz edit-command-line
zle -N edit-command-line
bindkey '^[e' edit-command-line

# --- Quiet Ayu — поиск и подсветка ввода ---
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
# При повторном source оставляем только выбранный prompt provider.
autoload -Uz add-zsh-hook
add-zsh-hook -d precmd prompt_starship_precmd
add-zsh-hook -d preexec prompt_starship_preexec
add-zsh-hook -d precmd _dots-prompt-precmd
add-zsh-hook -d preexec _dots-prompt-preexec
if [[ "${DOTS_USE_STARSHIP:-0}" == 1 ]]; then
  _starship_cache="$HOME/.cache/starship-init.zsh"
  if [[ ! -s "$_starship_cache" || "$(command -v starship)" -nt "$_starship_cache" ]]; then
    mkdir -p "$HOME/.cache"
    starship init zsh --print-full-init > "$_starship_cache" 2>/dev/null
  fi
  [[ -s "$_starship_cache" ]] && source "$_starship_cache"
  unset _starship_cache
else
  [[ "${RPROMPT-}" == '$('*starship*' prompt --right '*')' ]] && RPROMPT=
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
