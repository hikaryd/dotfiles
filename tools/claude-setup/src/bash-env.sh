claude() {
    command "${BASH_SOURCE[0]%/.local/share/claude-guard/runtime/bash-env.sh}/.local/bin/claude" "$@"
}
