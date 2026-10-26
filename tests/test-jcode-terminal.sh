#!/usr/bin/env bash
set -euo pipefail
unset JCODE_MERMAID_PICKER_PROBE OUTER_FEATURES PASSTHROUGH
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/jcode-terminal.XXXXXX")
trap 'rm -rf "$work"' EXIT
export HOME="$work/home"
mkdir -p "$HOME/.local/bin" "$HOME/.jcode/builds/stable" "$HOME/.config/jcode"
cat > "$HOME/.local/bin/dots-tools" <<'SH'
#!/bin/sh
[ "$*" = 'proxy pi --ensure' ] || exit 97
SH
cat > "$HOME/.local/bin/rmux" <<'SH'
#!/bin/sh
case "$1" in
  display-message) printf '%s' "${OUTER_FEATURES:-xterm-ghostty|RGB,kitty-graphics,sync}" ;;
  show-options) [ "$2" = -Awqv ] || exit 99; printf '%s' "${PASSTHROUGH:-on}" ;;
  *) exit 98 ;;
esac
SH
cat > "$HOME/.jcode/builds/stable/jcode" <<'SH'
#!/bin/sh
printf 'TERM_PROGRAM=%s\nPROBE=%s\nRMUX=%s\nTMUX=%s\nHERDR=%s\n' "${TERM_PROGRAM:-}" "${JCODE_MERMAID_PICKER_PROBE-unset}" "${RMUX:-}" "${TMUX:-}" "${HERDR_ENV-unset}"
printf 'ARG=<%s>\n' "$@"
SH
chmod +x "$HOME/.local/bin/"* "$HOME/.jcode/builds/stable/jcode"
export PATH="$HOME/.local/bin:/usr/bin:/bin"
check() { grep -Fx "$1" "$work/output" >/dev/null || { cat "$work/output" >&2; exit 1; }; }
export RMUX=/private/rmux,42,1 RMUX_PANE=%1 TMUX=/private/rmux,42,1 TERM_PROGRAM=rmux HERDR_ENV=1
sh "$root/config/jcode/launch" 'arg with spaces' > "$work/output"
check TERM_PROGRAM=ghostty
check PROBE=0
check HERDR=unset
check RMUX=/private/rmux,42,1
check TMUX=/private/rmux,42,1
check 'ARG=<arg with spaces>'
JCODE_MERMAID_PICKER_PROBE=1 sh "$root/config/jcode/launch" > "$work/output"
check PROBE=1
OUTER_FEATURES='xterm-256color|RGB' sh "$root/config/jcode/launch" > "$work/output"
check TERM_PROGRAM=rmux
check PROBE=unset
PASSTHROUGH=off sh "$root/config/jcode/launch" > "$work/output"
check TERM_PROGRAM=rmux
check PROBE=unset
unset RMUX RMUX_PANE TMUX
TERM_PROGRAM=ghostty sh "$root/config/jcode/launch" > "$work/output"
check TERM_PROGRAM=ghostty
check PROBE=unset
printf 'PASS Jcode image environment: verified Ghostty capabilities, mux identity, safe fallback and explicit override\n'
