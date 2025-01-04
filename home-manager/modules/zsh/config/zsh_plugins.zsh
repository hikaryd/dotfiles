#!/bin/zsh
function zcompile-many() {
	autoload -U zrecompile
	local f
	for f in "$@"; do zrecompile -pq "$f"; done
}

export XDG_CACHE_HOME="$HOME"/.cache
# For rebuildiing in case of updates.
function rebuild() {
	echo "starting compile"
	# Compile Powerlevel10k package
	make -sC $ZDOTDIR/plugins/powerlevel10k pkg zwc &

	# Move and compile Fast Syntax Highlighting scripts
	zcompile-many $ZDOTDIR/plugins/fast-syntax-highlighting/{fast*,.fast*,**/*.ch,**/*.zsh} &

	# Compile Zsh Autosuggestions scripts
	zcompile-many $ZDOTDIR/plugins/zsh-autosuggestions/{*.zsh,src/**/*.zsh} &

	# fzf-tab and friends.
	zcompile-many $ZDOTDIR/plugins/zsh-completions/{*.zsh,src/*} &
	zcompile-many $ZDOTDIR/plugins/fzf-tab/*.zsh &
	zcompile-many $ZDOTDIR/plugins/fzf-tab-source/{*.zsh,sources/*.zsh,functions/*.zsh} &
	zcompile-many $ZDOTDIR/init_atuin.zsh &

	#compinit
	zcompile-many $XDG_CACHE_HOME/zsh/^(*.(zwc|old)) &

	#and some more files too
	zcompile-many $ZDOTDIR/zsh_plugins.sh &
	wait
}

# Clone and compile to wordcode missing plugins.
plug() {
    # Extract the repository name from the URL
	# The ${1:t} extracts the tail of the URL, which is typically the part after the last / (i.e., repo.git).
	# The ${:r} removes the file extension, leaving the repository name (i.e., repo).
    local repo_name="${1:t:r}"
    # Clone the repository into the home directory under a directory named after the repo
    git clone --depth=1 "$1" "$ZDOTDIR"/plugins/"$repo_name"
	# Set the rebuild flag
	requires_rebuild=True
}

if [[ ! -e $ZDOTDIR/plugins/fast-syntax-highlighting ]]; then
	plug https://github.com/zdharma-continuum/fast-syntax-highlighting.git
	curl --create-dirs -O --output-dir ${XDG_CONFIG_HOME}/fsh https://raw.githubusercontent.com/catppuccin/zsh-fsh/main/themes/catppuccin-mocha.ini # catppucin gang rise up !
	source $ZDOTDIR/plugins/fast-syntax-highlighting/fast-syntax-highlighting.plugin.zsh # TODO: avoid sourcing twice
	fast-theme XDG:catppuccin-mocha
fi
if [[ ! -e $ZDOTDIR/plugins/zsh-autosuggestions ]]; then
	plug https://github.com/zsh-users/zsh-autosuggestions.git
fi
if [[ ! -e $ZDOTDIR/plugins/powerlevel10k ]]; then
	plug https://github.com/romkatv/powerlevel10k.git
fi
if [[ ! -e $ZDOTDIR/plugins/fzf-tab ]]; then
	plug https://github.com/Aloxaf/fzf-tab.git
fi
if [[ ! -e $ZDOTDIR/plugins/fzf-tab-source ]]; then
	plug https://github.com/Magniquick/fzf-tab-source.git
fi
if [[ ! -e $ZDOTDIR/plugins/zsh-completions ]]; then
	plug https://github.com/zsh-users/zsh-completions.git
fi
if [[ ! -e $ZDOTDIR/plugins/clipboard ]]; then
	plug https://github.com/zpm-zsh/clipboard.git
fi
if [[ ! -e $ZDOTDIR/plugins/evalcache ]]; then
	plug https://github.com/mroth/evalcache.git
fi
if [[ ! -e $ZDOTDIR/plugins/iTerm2-shell-integration ]] && [[ $TERM_PROGRAM = "iTerm.app" ]]; then
	plug https://github.com/gnachman/iTerm2-shell-integration.git
fi
unfunction plug

if ((${+requires_rebuild})); then
	rebuild
fi

# Load plugins.
# https://github.com/Aloxaf/fzf-tab?tab=readme-ov-file#install -> fzf-tab after compinit, before zsh plugins which warp widgets
fpath=($ZDOTDIR/plugins/zsh-completions/src $fpath)
# Enable the "new" completion system (compsys).
# Add plugins that change fpath above this - I nearly lost my sanity over this :/
if [ ! -d "$XDG_CACHE_HOME/zsh" ]; then mkdir -p "$XDG_CACHE_HOME/zsh"; fi
zstyle ':completion::complete:*' use-cache on
zstyle ':completion::complete:*' cache-path "${XDG_CACHE_HOME}/zsh/"
autoload -Uz compinit && compinit -u -d "${XDG_CACHE_HOME}/zsh/.zcompdump"

source $ZDOTDIR/plugins/clipboard/clipboard.plugin.zsh

if ! (($+commands[fzf])); then
	echo 'Warning: fzf is requried for the fzf-tab plugin but was not found.'
else
	export LESSOPEN="|$ZDOTDIR/lessfilter %s"
	setopt globdots
	zstyle ':completion:*' menu no
	zstyle ':fzf-tab:*' fzf-min-height 70
	zstyle ':completion:complete:*:argument-rest' sort false
	zstyle ':completion:*' matcher-list 'm:{[:lower:]}={[:upper:]}'  # case insensitive file matching
	zstyle ':completion:*' file-sort modification
	# switch group using `,`
	zstyle ':fzf-tab:*' switch-group ','
	# set list-colors to enable filename colorizing
	zstyle ':completion:*' list-colors ${(s.:.)LS_COLORS}
	if [[ $OSTYPE != "msys" ]]; then # absolutly broken on msys2
		source $ZDOTDIR/plugins/fzf-tab-source/fzf-tab-source.plugin.zsh
	fi
	source $ZDOTDIR/plugins/fzf-tab/fzf-tab.plugin.zsh
fi

ZSH_AUTOSUGGEST_MANUAL_REBIND=1
zle_highlight+=(paste:none)
source $ZDOTDIR/plugins/fast-syntax-highlighting/fast-syntax-highlighting.plugin.zsh
source $ZDOTDIR/plugins/zsh-autosuggestions/zsh-autosuggestions.zsh
ZSH_EVALCACHE_DIR="$XDG_CACHE_HOME"/zsh-evalcache
source $ZDOTDIR/plugins/evalcache/evalcache.plugin.zsh

if [[ $TERM = "xterm-kitty" ]] && [[ -n $KITTY_INSTALLATION_DIR ]]; then
	export KITTY_SHELL_INTEGRATION="enabled"
	autoload -Uz -- "$KITTY_INSTALLATION_DIR"/shell-integration/zsh/kitty-integration
	kitty-integration
	unfunction kitty-integration
elif [[ $TERM_PROGRAM = "iTerm.app" ]]; then
	source "$ZDOTDIR/plugins/iTerm2-shell-integration/shell_integration/zsh"
fi
