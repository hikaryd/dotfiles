#!/usr/bin/env bash
# Exercise the real GUI adapter without attaching to the developer's session.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/dots-terminal-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
export HOME="$work/home with spaces" PATH=/usr/bin:/bin
mkdir -p "$HOME/.local/bin"
cat > "$HOME/.local/bin/herdr" <<'SH'
#!/bin/sh
/usr/bin/env | /usr/bin/sort
printf 'HERDR_ARG=<%s>\n' "$@"
exit "${LAUNCH_EXIT:-0}"
SH
chmod +x "$HOME/.local/bin/herdr"
export HERDR_ENV=old HERDR_SOCKET_PATH=/old/socket HERDR_PANE_ID=old HERDR_ACTIVE_PANE_ID=old TMUX=old KEEP_ME=yes TERM=ghostty
sh "$root/scripts/dots-terminal" --session 'project with spaces' > "$work/output"
if grep -Eq '^(HERDR_(ENV|SOCKET_PATH|PANE_ID|ACTIVE_PANE_ID)=|TMUX=)' "$work/output"; then
  printf 'FAIL GUI launcher retained inherited pane context\n' >&2
  exit 1
fi
grep -Fx 'HERDR_ARG=<--session>' "$work/output"
grep -Fx 'HERDR_ARG=<project with spaces>' "$work/output"
grep -Fx 'KEEP_ME=yes' "$work/output"
grep -Fx 'TERM=ghostty' "$work/output"
grep -Fx "HOME=$HOME" "$work/output"
set +e
LAUNCH_EXIT=23 sh "$root/scripts/dots-terminal" > "$work/output"
status=$?
set -e
[[ $status -eq 23 ]]
printf 'PASS Herdr GUI launcher: clean context, quoted arguments, environment and exit status\n'
