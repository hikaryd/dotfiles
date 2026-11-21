# Resolve Claude explicitly even when another tool prepends its bin directory.
case "${PATH-}" in
  "$HOME/.local/bin"|"$HOME/.local/bin":*) ;;
  *) export PATH="$HOME/.local/bin:${PATH:-/usr/bin:/bin}" ;;
esac
unalias claude 2>/dev/null || :
claude() { "$HOME/.local/bin/claude" "$@"; }
unalias agy 2>/dev/null || :
agy() { "$HOME/.local/bin/claude-setup" agy "$@"; }
