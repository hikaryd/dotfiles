#!/bin/zsh
# make paths unique
typeset -U PATH FPATH path fpath

source $ZDOTDIR/env.zsh
source $ZDOTDIR/functions.zsh
source $ZDOTDIR/alias.zsh

# Auto cd !
setopt autocd
setopt interactivecomments
setopt histignorespace # better history.
setopt INC_APPEND_HISTORY # autosuggestions should work better
setopt EXTENDED_GLOB # required for the rebuild function to work, plus it's nice !

# python go brrr
export PATH="$HOME/.bun/bin:$HOME/.cargo/bin:$PATH:$HOME/.local/bin/:$HOME"

export PROMPT_EOL_MARK=""
export EDITOR="nvim"
export VISUAL="$EDITOR"

# Arrow keys traverses history considiring inital input buffer
bindkey "^[[A" history-beginning-search-backward
bindkey "^[[B" history-beginning-search-forward


bindkey '^H' backward-kill-word
bindkey '^[[3;5~' kill-word
bindkey '^[[3~' delete-char
bindkey '^[[1;5D' backward-word
bindkey '^[[1;5C' forward-word
bindkey '^Z' push-input

# init zoxide
_evalcache zoxide init zsh

if (($+commands[atuin])); then
	_evalcache atuin init zsh --disable-up-arrow
	_zsh_autosuggest_strategy_atuin_top() {
		suggestion=$(ATUIN_QUERY="$1" atuin search --cmd-only -e 0 --limit 1 --search-mode prefix)
	}
	_zsh_autosuggest_strategy_atuin_currentdir() {
		suggestion=$(ATUIN_QUERY="$1" atuin search --cmd-only -e 0 --limit 1 -c "$PWD" --search-mode prefix)
	}
fi

# bun completions
[ -s "/home/hikary/.bun/_bun" ] && source "/home/hikary/.bun/_bun"

eval "$(starship init zsh)"
