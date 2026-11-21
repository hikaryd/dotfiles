#!/bin/bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
t=$(mktemp -d "$root/.agy-test.XXXXXXXX")
trap 'rm -rf "$t"' EXIT
export HOME="$t/home" PATH="$t/bin:/usr/bin:/bin:/usr/sbin:/sbin" LOG="$t/log"
mkdir -p "$HOME" "$t/bin"
printf 'user settings\n' > "$HOME/.zshrc"
cat > "$t/bin/curl" <<'MOCK'
#!/bin/bash
while [[ $1 != -o ]]; do shift; done
cat > "$2" <<'INSTALL'
#!/bin/bash
[[ "$1" = --dir ]]
printf '#!/bin/bash\nexit 0\n' > "$2/agy"
chmod +x "$2/agy"
printf 'unwanted\n' >> "$HOME/.zshrc"
INSTALL
MOCK
cat > "$t/bin/launchctl" <<'MOCK'
#!/bin/bash
echo "launchctl $*" >> "$LOG"
[[ $1 = print ]] && exit "${LOADED:-1}"
exit 0
MOCK
cat > "$t/bin/pgrep" <<'MOCK'
#!/bin/bash
exit "${RUNNING:-1}"
MOCK
cat > "$t/bin/brew" <<'MOCK'
#!/bin/bash
echo "brew $*" >> "$LOG"
[[ $1 = list ]] && exit "${INSTALLED:-0}"
exit "${UPGRADE_STATUS:-0}"
MOCK
chmod +x "$t/bin/"*
bash "$root/scripts/agy-install.sh"
[[ -x "$HOME/.local/bin/agy" && $(cat "$HOME/.zshrc") = 'user settings' ]]
printf 'existing\n' > "$HOME/.local/bin/agy"
bash "$root/scripts/agy-install.sh"
[[ $(cat "$HOME/.local/bin/agy") = existing ]]
rm "$HOME/.local/bin/agy"
ln -s missing "$HOME/.local/bin/agy"
bash "$root/scripts/agy-install.sh"
[[ -L "$HOME/.local/bin/agy" ]]
rm "$HOME/.local/bin/agy"
printf '#!/bin/bash\nexit 1\n' > "$t/bin/curl"
chmod +x "$t/bin/curl"
if bash "$root/scripts/agy-install.sh"; then exit 1; fi
[[ ! -e "$HOME/.local/bin/agy" ]]
bash "$root/scripts/agy-install-schedule.sh"
bash "$root/scripts/agy-install-schedule.sh"
[[ $(grep -c 'launchctl bootstrap' "$LOG") = 2 ]]
printf 'foreign' > "$HOME/Library/LaunchAgents/local.dots.antigravity-update.plist"
if bash "$root/scripts/agy-install-schedule.sh" 2>/dev/null; then exit 1; fi
[[ $(cat "$HOME/Library/LaunchAgents/local.dots.antigravity-update.plist") = foreign ]]
: > "$LOG"
LOADED=0 bash "$root/scripts/agy-install-update.sh"
! grep -q '^brew ' "$LOG"
RUNNING=0 bash "$root/scripts/agy-install-update.sh"
! grep -q '^brew ' "$LOG"
INSTALLED=1 bash "$root/scripts/agy-install-update.sh"
! grep -q 'brew upgrade' "$LOG"
bash "$root/scripts/agy-install-update.sh"
grep -q '^brew upgrade --cask lbjlaq/antigravity-manager/antigravity-tools$' "$LOG"
if UPGRADE_STATUS=1 bash "$root/scripts/agy-install-update.sh"; then exit 1; fi
printf 'all agy setup tests passed\n'
