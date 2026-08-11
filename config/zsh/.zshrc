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

# Podbor Kubernetes environments.
# Wrappers always pin both kubeconfig and context, so commands cannot accidentally
# fall through to another environment from the merged default KUBECONFIG.
export PODBOR_DEV_KUBECONFIG="$HOME/.kube/configs/default/kubeconfig-kube-podbor-dev-2-07368e16-s4.yaml"
export PODBOR_DEV_CONTEXT="ats-dev"
export PODBOR_DEV_NAMESPACE="podbor-dev"

export PODBOR_STAGE_KUBECONFIG="$HOME/.kube/configs/default/podbor-stage.yaml"
export PODBOR_STAGE_CONTEXT="aihr-stage"
export PODBOR_STAGE_NAMESPACE="podbor-stage"

export PODBOR_PROD_KUBECONFIG="$HOME/.kube/configs/prod/podbor-prod.yaml"
export PODBOR_PROD_CONTEXT="cluster-admin@cluster"
export PODBOR_PROD_NAMESPACE="podbor-prod"

_podbor_kubectl() {
  local kubeconfig="$1"
  local context="$2"
  local namespace="$3"
  shift 3

  if [[ ! -r "$kubeconfig" ]]; then
    echo "Kubeconfig is not readable: $kubeconfig" >&2
    return 1
  fi

  local -a namespace_arg=()
  [[ -n "$namespace" ]] && namespace_arg=(--namespace="$namespace")

  KUBECONFIG="$kubeconfig" \
    kubectl --context="$context" "${namespace_arg[@]}" "$@"
}

_podbor_prod_confirm() {
  [[ "$KPROD_CONFIRMED" = "1" ]] && return 0

  printf "⚠️  PROD MUTATING COMMAND — type 'yes' to proceed: "
  local answer
  read -r answer
  [[ "$answer" = "yes" ]] || {
    echo "Aborted."
    return 1
  }
}

_podbor_kubectl_is_read_only() {
  local command="${1:-}"
  local subcommand="${2:-}"

  case "$command" in
    get|describe|logs|top|events|explain|api-resources|api-versions|cluster-info|version|diff|wait)
      return 0
      ;;
    rollout)
      [[ "$subcommand" = "status" || "$subcommand" = "history" ]]
      return
      ;;
    auth)
      [[ "$subcommand" = "can-i" || "$subcommand" = "whoami" ]]
      return
      ;;
    *)
      return 1
      ;;
  esac
}

_podbor_prod_confirm_if_mutating() {
  _podbor_kubectl_is_read_only "$@" && return 0
  _podbor_prod_confirm
}

# Cluster-wide kubectl wrappers.
kdev() {
  _podbor_kubectl "$PODBOR_DEV_KUBECONFIG" "$PODBOR_DEV_CONTEXT" "" "$@"
}

kstage() {
  _podbor_kubectl "$PODBOR_STAGE_KUBECONFIG" "$PODBOR_STAGE_CONTEXT" "" "$@"
}

kprod() {
  _podbor_prod_confirm_if_mutating "$@" || return
  _podbor_kubectl "$PODBOR_PROD_KUBECONFIG" "$PODBOR_PROD_CONTEXT" "" "$@"
}

# Namespace-pinned kubectl wrappers.
kpdev() {
  _podbor_kubectl \
    "$PODBOR_DEV_KUBECONFIG" "$PODBOR_DEV_CONTEXT" "$PODBOR_DEV_NAMESPACE" "$@"
}

kpstage() {
  _podbor_kubectl \
    "$PODBOR_STAGE_KUBECONFIG" "$PODBOR_STAGE_CONTEXT" "$PODBOR_STAGE_NAMESPACE" "$@"
}

kpprod() {
  _podbor_prod_confirm_if_mutating "$@" || return
  _podbor_kubectl \
    "$PODBOR_PROD_KUBECONFIG" "$PODBOR_PROD_CONTEXT" "$PODBOR_PROD_NAMESPACE" "$@"
}

# Interactive pod logs, manual CronJob runs, and rollout monitoring.
[[ -f "$HOME/dots/config/zsh/kube-tools.zsh" ]] &&
  source "$HOME/dots/config/zsh/kube-tools.zsh"

# Explicit connection checks; no network request is made until one is called.
kdev-connect() {
  kdev cluster-info
}

kstage-connect() {
  kstage cluster-info
}

kprod-connect() {
  kprod cluster-info
}

podbor-kube-help() {
  cat <<'EOF'
Podbor Kubernetes commands:
  kdev <args>       kubectl in ats-dev
  kstage <args>     kubectl in aihr-stage
  kprod <args>      kubectl in prod (confirmation only for mutations)

  kpdev <args>      kubectl in podbor-dev namespace
  kpstage <args>    kubectl in podbor-stage namespace
  kpprod <args>     kubectl in podbor-prod (confirmation only for mutations)

  kdev-connect      check dev connection
  kstage-connect    check stage connection
  kprod-connect     check prod connection

  kpdev-logs [text]       choose a pod and follow all container logs
  kpdev-logs-save [text]  choose a pod and save all available logs
  kpdev-cron-run [text]   ACTION: run CronJob; save logs to ~/Documents/kube-logs
  kpdev-exec              choose target, bash/Python, and working directory
  kpdev-exec [target] -- <command>  execute an explicit command
  kpdev-log-search [pod-pattern] [text]  search all matching pod logs
  kpdev-jobs [text]       choose a recent Job and follow all of its pod logs
  kpdev-jobs-save [text]  choose a recent Job and save all pod/container logs
  kpdev-deploy-watch [text]  watch a workload rollout; read-only
  kpdev-pod-analyze [text]   live CPU/RAM/status/events with local history charts
  kpdev-pod-restart [text]   ACTION: delete a pod, wait for replacement, follow logs

  Replace "dev" with "stage" or "prod" in all interactive commands.

Examples:
  kpdev get pods
  kpstage get cronjobs
  kpprod get deployments
  kpdev-logs worker
  kpdev-jobs-save request-creation
  kpstage-cron-run request-creation
  kpstage-exec
  kpstage-log-search 'podbor-api*' 'trace-id'
  kpstage-jobs security-checker
  kpdev-deploy-watch podbor-api
  kpstage-pod-analyze service-api
  kpstage-pod-restart service-api

Set KPROD_CONFIRMED=1 only for a deliberately pre-confirmed prod shell.
EOF
}

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
autoload -Uz extract graphify-merge-fix kafka-consume kafka-produce tp

# --- Key bindings ---
bindkey -e
bindkey '^A' beginning-of-line
bindkey '^E' end-of-line
bindkey '^P' autosuggest-accept
bindkey '^N' down-line-or-search

# --- Aliases ---
alias v='nvim'
alias cat='bat --style=plain'
alias y='yazi'
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

# Run OMX without the tmux HUD/status pane.
export OMX_LAUNCH_POLICY=direct
