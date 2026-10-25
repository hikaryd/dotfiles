#!/usr/bin/env bash
# Exercise the real GUI adapter without attaching to the developer's session.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/dots-terminal-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
export HOME="$work/home with spaces" PATH=/usr/bin:/bin
mkdir -p "$HOME/.local/bin"
mkdir -p "$HOME/.config/rmux"
touch "$HOME/.config/rmux/rmux.conf"
cat > "$HOME/.local/bin/herdr" <<'SH'
#!/bin/sh
/usr/bin/env | /usr/bin/sort
printf 'HERDR_ARG=<%s>\n' "$@"
exit "${LAUNCH_EXIT:-0}"
SH
cat > "$HOME/.local/bin/rmux" <<'SH'
#!/bin/sh
if [ "${1:-}" = has-session ]; then exit "${HAS_SESSION:-1}"; fi
if [ "${1:-}" = show-options ]; then printf '%s' "${PROFILE_MARKER:-}"; exit 0; fi
if [ "${1:-}" = source-file ]; then printf 'PROFILE_SOURCE=<%s>\n' "$2"; exit 0; fi
/usr/bin/env | /usr/bin/sort
printf 'RMUX_ARG=<%s>\n' "$@"
exit "${LAUNCH_EXIT:-0}"
SH
chmod +x "$HOME/.local/bin/herdr" "$HOME/.local/bin/rmux"
export DOTS_RMUX_BIN="$HOME/.local/bin/rmux"
export HERDR_ENV=old HERDR_SOCKET_PATH=/old/socket HERDR_PANE_ID=old HERDR_ACTIVE_PANE_ID=old
export TMUX=old TMUX_PANE=%99 TMUX_PROGRAM=/old/tmux RMUX=old RMUX_PANE=%99 KEEP_ME=yes TERM=ghostty
check_clean() {
  if grep -E '^(HERDR_|TMUX|RMUX)' "$work/output" | grep -v '_ARG='; then
    printf 'FAIL GUI launcher retained inherited pane context\n' >&2
    exit 1
  fi
  grep -Fx 'KEEP_ME=yes' "$work/output" >/dev/null
  grep -Fx 'TERM=ghostty' "$work/output" >/dev/null
  grep -Fx "HOME=$HOME" "$work/output" >/dev/null
}
sh "$root/scripts/dots-terminal" > "$work/output"
grep -Fx 'RMUX_ARG=<new-session>' "$work/output"
grep -Fx 'RMUX_ARG=<-A>' "$work/output"
grep -Fx 'RMUX_ARG=<main>' "$work/output"
check_clean
grep -Fx "PATH=$HOME/.local/bin:/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin" "$work/output" >/dev/null
HAS_SESSION=0 sh "$root/scripts/dots-terminal" > "$work/output"
grep -Fx 'RMUX_ARG=<attach-session>' "$work/output"
grep -Fx "PROFILE_SOURCE=<$HOME/.config/rmux/rmux.conf>" "$work/output"
HAS_SESSION=0 PROFILE_MARKER=dots-native-v1 sh "$root/scripts/dots-terminal" > "$work/output"
if grep -F 'PROFILE_SOURCE=' "$work/output"; then
  printf 'FAIL an active native profile was needlessly reloaded\n' >&2
  exit 1
fi
sh "$root/scripts/dots-terminal" --execute 'use ghostty *' > "$work/output"
grep -Fx 'RMUX_ARG=<new-session>' "$work/output"
if grep -F 'RMUX_ARG=<--execute>' "$work/output"; then
  printf 'FAIL Ghostty Nushell bootstrap was forwarded to rmux\n' >&2
  exit 1
fi
sh "$root/scripts/dots-terminal" new-session -s 'project with spaces' > "$work/output"
grep -Fx 'RMUX_ARG=<project with spaces>' "$work/output"
sh "$root/scripts/dots-terminal" --herdr --session 'old project with spaces' > "$work/output"
grep -Fx 'HERDR_ARG=<--session>' "$work/output"
grep -Fx 'HERDR_ARG=<old project with spaces>' "$work/output"
check_clean
DOTS_RMUX_BIN="$work/missing" sh "$root/scripts/dots-terminal" > "$work/output" 2> "$work/error"
grep -F 'Herdr' "$work/error"
grep -Fx 'HERDR_ARG=<>' "$work/output"
set +e
LAUNCH_EXIT=23 sh "$root/scripts/dots-terminal" > "$work/output"
status=$?
set -e
[[ $status -eq 23 ]]
printf 'PASS GUI launcher: rmux attach/create, preserved Herdr, clean context, arguments and exit status\n'
