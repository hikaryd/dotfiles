#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
if [[ $(uname -s) != Darwin || ( ! -d /Applications/Rex\ Beta.app && ! -d /Applications/Rex.app ) ]]; then
  echo 'SKIP Rex installer idempotence: macOS Rex app not installed'
  exit 0
fi
work=$(mktemp -d "${TMPDIR:-/tmp}/rex-installer-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/repo/scripts" "$work/repo/tools/rex-last-session" "$work/bin" "$work/home"
cp "$root/scripts/setup-rex-last-session.sh" "$work/repo/scripts/"
cp "$root/tools/rex-last-session/main.swift" "$work/repo/tools/rex-last-session/"
cat > "$work/bin/swiftc" <<'STUB'
#!/usr/bin/env bash
set -eu
echo swiftc >> "$TEST_LOG"
[[ ${FAIL_BUILD:-0} != 1 ]] || exit 1
printf '#!/bin/sh\n# %s\n' "$(shasum -a 256 "$1")" > "$3"
chmod +x "$3"
STUB
cat > "$work/bin/launchctl" <<'STUB'
#!/usr/bin/env bash
set -eu
echo "$1" >> "$TEST_LOG"
case "$1" in
  print)
    [[ -e "$TEST_AGENT" ]] || exit 1
    if [[ $(cat "$TEST_AGENT") == running ]]; then
      printf '\tstate = running\n\tpid = 12345\n'
    else
      printf '\tstate = exited\n\tlast exit code = 1\n'
    fi ;;
  bootout) rm -f "$TEST_AGENT" ;;
  bootstrap)
    if [[ ${FAIL_BOOTSTRAP:-0} == 1 ]]; then echo exited > "$TEST_AGENT"; exit 1; fi
    echo running > "$TEST_AGENT" ;;
  kickstart) echo running > "$TEST_AGENT" ;;
  *) exit 1 ;;
esac
STUB
chmod +x "$work/bin/swiftc" "$work/bin/launchctl"
export PATH="$work/bin:$PATH" HOME="$work/home" TEST_LOG="$work/log" TEST_AGENT="$work/agent"
script="$work/repo/scripts/setup-rex-last-session.sh"
binary="$HOME/.local/bin/dots-rex-last-session"
plist="$HOME/Library/LaunchAgents/local.dots.rex-last-session.plist"
marker="$binary.sha256"
run() { bash "$script" > "$work/output"; }
inode() { stat -f '%i' "$binary"; }
hash() { shasum -a 256 "$1" | cut -d ' ' -f 1; }
run
[[ -x $binary && -f $marker && -f $plist && -f $TEST_AGENT ]]
old_inode=$(inode); old_hash=$(hash "$binary"); old_plist=$(hash "$plist"); old_marker=$(hash "$marker")
: > "$TEST_LOG"
run
[[ $(inode) == "$old_inode" && $(hash "$binary") == "$old_hash" ]]
! grep -Eq '^(swiftc|bootout|bootstrap)$' "$TEST_LOG"
printf '\n// changed fixture\n' >> "$work/repo/tools/rex-last-session/main.swift"
: > "$TEST_LOG"
run
grep -Fx swiftc "$TEST_LOG" >/dev/null
grep -Fx bootout "$TEST_LOG" >/dev/null
[[ $(hash "$binary") != "$old_hash" ]]
old_inode=$(inode); old_hash=$(hash "$binary"); old_plist=$(hash "$plist"); old_marker=$(hash "$marker")
printf '\n// compile failure fixture\n' >> "$work/repo/tools/rex-last-session/main.swift"
: > "$TEST_LOG"
if FAIL_BUILD=1 run; then echo 'FAIL compiler failure accepted' >&2; exit 1; fi
[[ $(inode) == "$old_inode" && $(hash "$binary") == "$old_hash" && $(hash "$plist") == "$old_plist" && $(hash "$marker") == "$old_marker" && -f $TEST_AGENT ]]
! grep -Eq '^(bootout|bootstrap)$' "$TEST_LOG"
# Restore the source corresponding to the installed marker.
cp "$root/tools/rex-last-session/main.swift" "$work/repo/tools/rex-last-session/main.swift"
printf '\n// changed fixture\n' >> "$work/repo/tools/rex-last-session/main.swift"
echo exited > "$TEST_AGENT"
: > "$TEST_LOG"
run
[[ $(cat "$TEST_AGENT") == running && $(inode) == "$old_inode" ]]
grep -Fx kickstart "$TEST_LOG" >/dev/null
! grep -Eq '^(swiftc|bootout|bootstrap)$' "$TEST_LOG"
rm "$TEST_AGENT"
: > "$TEST_LOG"
if FAIL_BOOTSTRAP=1 run; then echo 'FAIL bootstrap failure accepted' >&2; exit 1; fi
[[ $(cat "$TEST_AGENT") == exited && $(inode) == "$old_inode" ]]
: > "$TEST_LOG"
run
[[ $(cat "$TEST_AGENT") == running && $(inode) == "$old_inode" ]]
grep -Fx kickstart "$TEST_LOG" >/dev/null
! grep -Eq '^(swiftc|bootout|bootstrap)$' "$TEST_LOG"
rm "$TEST_AGENT"
: > "$TEST_LOG"
run
[[ -f $TEST_AGENT && $(inode) == "$old_inode" ]]
grep -Fx bootstrap "$TEST_LOG" >/dev/null
! grep -Eq '^(swiftc|bootout)$' "$TEST_LOG"
printf 'invalid plist\n' > "$plist"
: > "$TEST_LOG"
run
[[ $(inode) == "$old_inode" && $(hash "$plist") == "$old_plist" ]]
! grep -Fx swiftc "$TEST_LOG" >/dev/null
rm "$binary"
: > "$TEST_LOG"
run
[[ -x $binary ]]
grep -Fx swiftc "$TEST_LOG" >/dev/null
printf '%s\n' 'PASS Rex installer idempotence'
