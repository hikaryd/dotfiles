#!/usr/bin/env bash
# Isolated shell contracts. Never connect to a real multiplexer.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/dots-tuios-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
export HOME="$work/home with spaces" XDG_DATA_HOME="$work/data" XDG_CONFIG_HOME="$work/config" XDG_STATE_HOME="$work/state" XDG_RUNTIME_DIR="$work/runtime"
export DOTS_TUIOS_PROFILE="$work/profile"
mkdir -p "$HOME/.local/bin" "$XDG_DATA_HOME/dots/tuios/0.11.0" "$DOTS_TUIOS_PROFILE"
cat > "$XDG_DATA_HOME/dots/tuios/0.11.0/tuios" <<'SH'
#!/bin/sh
printf 'arg=<%s>\n' "$@"
/usr/bin/env | /usr/bin/grep -E '^(XDG_|TUIOS_|HERDR_|TMUX|KEEP_ME|DOTS_TUIOS_XDG_)' | /usr/bin/sort
SH
chmod +x "$XDG_DATA_HOME/dots/tuios/0.11.0/tuios"
sh "$root/scripts/tuios" attach 'a b' > "$work/output"
grep -Fx 'arg=<a b>' "$work/output"
grep -Fx "XDG_RUNTIME_DIR=$work/profile/run" "$work/output"
grep -Fx "DOTS_TUIOS_XDG_CONFIG_HOME=$work/config" "$work/output"
grep -Fx 'TUIOS_SERVER=dots' "$work/output"
printf 'dots-trial\n' > "$DOTS_TUIOS_PROFILE/server"
sh "$root/scripts/tuios" ls > "$work/output"
grep -Fx 'TUIOS_SERVER=dots-trial' "$work/output"
printf 'PASS managed binary, exact argv and persistent profile\n'
cp "$XDG_DATA_HOME/dots/tuios/0.11.0/tuios" "$HOME/.local/bin/tuios"
export HERDR_ENV=old HERDR_SOCKET_PATH=/old/socket HERDR_PANE_ID=old TUIOS_ENV=old TUIOS_SOCKET=/old/tuios TUIOS_WINDOW_ID=old TMUX=old KEEP_ME=yes
sh "$root/scripts/dots-terminal" > "$work/output"
! grep -Eq '^(HERDR_|TUIOS_|TMUX)' "$work/output"
grep -Fx 'KEEP_ME=yes' "$work/output"
printf 'PASS outer terminal never inherits pane authority\n'
cat > "$HOME/.local/bin/dots-nu" <<'SH'
#!/bin/sh
/usr/bin/env | /usr/bin/grep -E '^(XDG_|HERDR_|TUIOS_)' | /usr/bin/sort
printf 'arg=<%s>\n' "$@"
SH
chmod +x "$HOME/.local/bin/dots-nu"
export DOTS_TUIOS_XDG_CONFIG_HOME="$work/original config" DOTS_TUIOS_XDG_STATE_HOME="$work/original state" DOTS_TUIOS_XDG_RUNTIME_DIR=''
sh "$root/scripts/dots-tuios-shell" '--test arg' > "$work/output"
grep -Fx "XDG_CONFIG_HOME=$work/original config" "$work/output"
grep -Fx "XDG_STATE_HOME=$work/original state" "$work/output"
! grep -q '^XDG_RUNTIME_DIR=' "$work/output"
grep -Fx 'HERDR_SOCKET_PATH=/old/socket' "$work/output"
grep -Fx 'arg=<--test arg>' "$work/output"
printf 'PASS Nushell keeps fresh mux identity and ordinary application directories\n'
unset HERDR_ENV HERDR_SOCKET_PATH HERDR_PANE_ID TUIOS_ENV TUIOS_SOCKET TUIOS_WINDOW_ID TMUX
printf '#!/bin/sh\nprintf "tuios version 0.11.0 [pure-Go backend]\\n"\n' > "$XDG_DATA_HOME/dots/tuios/0.11.0/tuios"
chmod +x "$XDG_DATA_HOME/dots/tuios/0.11.0/tuios"
printf 'legacy executable\n' > "$HOME/.local/bin/tuios"
mkdir -p "$XDG_CONFIG_HOME/tuios"
printf 'legacy config\n' > "$XDG_CONFIG_HOME/tuios/config.toml"
sh "$root/scripts/install-tuios.sh"
[[ -L "$HOME/.local/bin/tuios" && $(readlink "$HOME/.local/bin/tuios") == "$root/scripts/tuios" ]]
grep -Fx 'legacy executable' "$HOME"/.config/dots-backups/tuios/*/bin/tuios
grep -Fx 'legacy config' "$HOME"/.config/dots-backups/tuios/*/config/tuios/config.toml
[[ -L "$DOTS_TUIOS_PROFILE/config/tuios" ]]
before=$(find "$HOME/.config/dots-backups/tuios" -type f | wc -l)
sh "$root/scripts/install-tuios.sh"
[[ $(find "$HOME/.config/dots-backups/tuios" -type f | wc -l) == "$before" ]]
printf 'PASS installer preserves legacy binary and is idempotent\n'
