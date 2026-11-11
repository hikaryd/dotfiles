#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
(
  uname() { echo Linux; }
  swiftc() { echo 'unexpected compiler invocation' >&2; return 1; }
  export -f uname swiftc
  bash scripts/setup-rex-last-session.sh | grep -F 'skipping.'
)
if [[ $(uname -s) == Darwin ]]; then
  swift_sandbox=$(mktemp -d "${TMPDIR:-/tmp}/rex-history-test.XXXXXX")
  trap 'rm -rf "$swift_sandbox"' EXIT
  swiftc -D TEST tools/rex-last-session/main.swift -o "$swift_sandbox/selftest"
  REX_TEST_DIR="$swift_sandbox/state" "$swift_sandbox/selftest"
fi
"${LUA_BIN:-lua}" tests/test-rex.lua
"${NU_BIN:-nu}" --no-config-file -c 'source config/nushell/config.nu; if ($env.PATH | first) == "" { error make {msg: "empty PATH"} }'
REX_BIN_DIR='/rex server/Current/bin' "${NU_BIN:-nu}" --no-config-file -c 'source config/nushell/config.nu; if ($env.PATH | first) != $env.REX_BIN_DIR { error make {msg: "Rex PATH precedence lost"} }'
if [[ -d /Applications/Rex\ Beta.app || -d /Applications/Rex.app ]]; then
  sandbox=$(mktemp -d "${TMPDIR:-/tmp}/rex-prefs-test.XXXXXX")
  trap 'rm -rf "$sandbox" "${swift_sandbox:-}"' EXIT
  log="$sandbox/preferences"
  defaults() { printf '%s\n' "$*" >> "$log"; }
  export -f defaults
  export log
  export REX_CALLS="$sandbox/rex-calls"
  home="$sandbox/home with spaces"
  mkdir -p "$home/.local/bin" "$home/.config/rex"
  if HOME="$home" bash scripts/setup-rex.sh > "$sandbox/missing-launcher" 2>&1; then
    printf '%s\n' 'FAIL missing launcher was accepted' >&2
    exit 1
  fi
  grep -F 'Rex preferences unchanged:' "$sandbox/missing-launcher"
  [[ ! -e "$log" ]]
  touch "$home/.local/bin/dots-tools"
  chmod +x "$home/.local/bin/dots-tools"
  touch "$home/.config/rex/ayu-dark"
  cat > "$home/.local/bin/rex" <<'STUB'
#!/usr/bin/env bash
if [[ "$*" == 'client ls --json' ]]; then
  printf '%s\n' '{"clients":[{"client_id":"client:test","info":{"kind":"app","platform":"macos"},"connection_state":"connected"}]}'
else
  printf '%s\n' "$*" >> "$REX_CALLS"
  if [[ "$*" == *'client.theme.import '* ]]; then exit "${REX_IMPORT_EXIT:-0}"; fi
  if [[ "$*" == *'client.theme.change '* ]]; then exit "${REX_SELECT_EXIT:-0}"; fi
fi
STUB
  chmod +x "$home/.local/bin/rex"
  HOME="$home" bash scripts/setup-rex.sh
  grep -Fx "write com.superlogical.rex Workspace.terminalStartupCommand -string \"$home/.local/bin/dots-tools\" desktop nu" "$log"
  grep -Fx 'write com.superlogical.rex Workspace.terminalFontPointSize -float 15' "$log"
  grep -Fx 'write com.superlogical.rex Workspace.terminalFontName -string HackNFM-Regular' "$log"
  grep -Fx 'write com.superlogical.rex Workspace.terminalFallbackFontNames -array' "$log"
  grep -Fx 'write com.superlogical.rex Workspace.tabStyle -string horizontal' "$log"
  grep -Fx 'write com.superlogical.rex Workspace.appearance -string dark' "$log"
  grep -Fx 'write com.superlogical.rex Workspace.interfaceStyle -string blendTheme' "$log"
  grep -Fx 'write com.superlogical.rex Workspace.usesPaneGlass -bool true' "$log"
  grep -Fx 'write com.superlogical.rex Workspace.showsPaneHeaders -bool false' "$log"
  grep -Fx 'write com.superlogical.rex Workspace.terminalImproveTextContrast -bool true' "$log"
  grep -Fx 'write com.superlogical.rex Workspace.terminalCellHeightPercent -float 12' "$log"
  grep -Fx 'write com.superlogical.rex Workspace.notificationPosition -string top-right' "$log"
  grep -Fx "do -C client:test client.theme.import path=$home/.config/rex/ayu-dark" "$REX_CALLS"
  grep -Fx 'do -C client:test client.appearance.switch_mode mode=dark' "$REX_CALLS"
  grep -Fx 'do -C client:test client.theme.change name=ayu-dark' "$REX_CALLS"
  : > "$REX_CALLS"
  HOME="$home" REX_IMPORT_EXIT=1 bash scripts/setup-rex.sh | grep -F 'Ayu setup pending:'
  [[ $(wc -l < "$REX_CALLS" | tr -d ' ') == 1 ]]
  HOME="$home" REX_SELECT_EXIT=1 bash scripts/setup-rex.sh > "$sandbox/select-failed"
  grep -F 'Ayu setup pending:' "$sandbox/select-failed"
  if grep -F 'theme imported and selected.' "$sandbox/select-failed"; then exit 1; fi
  if grep -E 'remoteControl|Tailscale|automatic-restart' "$log"; then exit 1; fi
else
  printf '%s\n' 'SKIP Rex preference writes: macOS Rex app not installed'
fi
printf '%s\n' 'PASS Rex shell and preferences contract'
