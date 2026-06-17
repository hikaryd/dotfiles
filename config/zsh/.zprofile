# Homebrew environment, kept static to avoid spawning `brew shellenv` on every
# login shell startup.
export HOMEBREW_PREFIX="/opt/homebrew"
export HOMEBREW_CELLAR="/opt/homebrew/Cellar"
export HOMEBREW_REPOSITORY="/opt/homebrew"
export INFOPATH="$HOMEBREW_PREFIX/share/info:${INFOPATH:-}"

typeset -U path
path=("$HOMEBREW_PREFIX/bin" "$HOMEBREW_PREFIX/sbin" $path)
