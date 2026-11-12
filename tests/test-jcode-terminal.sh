#!/usr/bin/env bash
set -euo pipefail
unset JCODE_MERMAID_PICKER_PROBE
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/jcode-terminal.XXXXXX")
trap 'rm -rf "$work"' EXIT
export HOME="$work/home"
mkdir -p "$HOME/.local/bin" "$HOME/.jcode/builds/stable" "$HOME/.config/jcode"
cat > "$HOME/.local/bin/dots-tools" <<'SH'
#!/bin/sh
[ "$*" = 'proxy pi --ensure' ] || exit 97
SH
cat > "$HOME/.jcode/builds/stable/jcode" <<'SH'
#!/bin/sh
printf 'TERM_PROGRAM=%s\nPROBE=%s\nTMUX=%s\n' "${TERM_PROGRAM:-}" "${JCODE_MERMAID_PICKER_PROBE-unset}" "${TMUX:-}"
printf 'ARG=<%s>\n' "$@"
SH
chmod +x "$HOME/.local/bin/"* "$HOME/.jcode/builds/stable/jcode"
export PATH="$HOME/.local/bin:/usr/bin:/bin"
check() { grep -Fx "$1" "$work/output" >/dev/null || { cat "$work/output" >&2; exit 1; }; }
# Jcode handles tmux passthrough itself; the launcher must not rewrite terminal identity.
export TMUX=/private/tmp/tmux-1/default,42,1 TMUX_PANE=%1 TERM_PROGRAM=tmux
sh "$root/config/jcode/launch" 'arg with spaces' > "$work/output"
check TERM_PROGRAM=tmux
check PROBE=unset
check TMUX=/private/tmp/tmux-1/default,42,1
check 'ARG=<arg with spaces>'
JCODE_MERMAID_PICKER_PROBE=1 sh "$root/config/jcode/launch" > "$work/output"
check PROBE=1
printf 'PASS Jcode environment: tmux identity and explicit overrides pass through unchanged\n'
