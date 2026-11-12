#!/usr/bin/env bash
# Exercise the real GUI adapter without attaching to the developer's session.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/dots-terminal-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
export HOME="$work/home with spaces" PATH=/usr/bin:/bin
mkdir -p "$HOME/.local/bin"
cat > "$HOME/.local/bin/tmux" <<'SH'
#!/bin/sh
if [ "${1:-}" = has-session ]; then exit "${HAS_SESSION:-1}"; fi
/usr/bin/env | /usr/bin/sort
printf 'TMUX_ARG=<%s>\n' "$@"
exit "${LAUNCH_EXIT:-0}"
SH
chmod +x "$HOME/.local/bin/tmux"
export TMUX=old TMUX_PANE=%99 TMUX_PROGRAM=/old/tmux DOTS_SOURCE_PANE=%1 DOTS_MUX_CLIENT=/dev/ttys001 KEEP_ME=yes TERM=ghostty
check_clean() {
  if grep -E '^(TMUX|DOTS_SOURCE_PANE|DOTS_MUX_CLIENT)' "$work/output" | grep -v '_ARG='; then
    printf 'FAIL GUI launcher retained inherited pane context\n' >&2
    exit 1
  fi
  grep -Fx 'KEEP_ME=yes' "$work/output" >/dev/null
  grep -Fx 'TERM=ghostty' "$work/output" >/dev/null
  grep -Fx "HOME=$HOME" "$work/output" >/dev/null
}
sh "$root/scripts/dots-terminal" > "$work/output"
grep -Fx 'TMUX_ARG=<new-session>' "$work/output"
grep -Fx 'TMUX_ARG=<main>' "$work/output"
check_clean
grep -Fx "PATH=$HOME/.local/bin:/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin" "$work/output" >/dev/null
HAS_SESSION=0 sh "$root/scripts/dots-terminal" > "$work/output"
grep -Fx 'TMUX_ARG=<attach-session>' "$work/output"
if grep -F 'TMUX_ARG=<main>' "$work/output"; then printf 'FAIL live server got an extra main session\n' >&2; exit 1; fi
check_clean
sh "$root/scripts/dots-terminal" --execute 'use ghostty *' > "$work/output"
grep -Fx 'TMUX_ARG=<new-session>' "$work/output"
if grep -F 'TMUX_ARG=<--execute>' "$work/output"; then
  printf 'FAIL Ghostty Nushell bootstrap was forwarded to tmux\n' >&2
  exit 1
fi
sh "$root/scripts/dots-terminal" new-session -s 'literal ; spaces' > "$work/output"
grep -Fx 'TMUX_ARG=<literal ; spaces>' "$work/output"
set +e
LAUNCH_EXIT=23 sh "$root/scripts/dots-terminal" > "$work/output"
status=$?
set -e
[[ $status -eq 23 ]]
printf 'PASS GUI launcher: tmux attach/create, clean context, arguments and exit status\n'
