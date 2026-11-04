#!/usr/bin/env bash
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
"${LUA_BIN:-lua}" tests/test-rex.lua
"${NU_BIN:-nu}" --no-config-file -c 'source config/nushell/config.nu; if ($env.PATH | first) == "" { error make {msg: "empty PATH"} }'
REX_BIN_DIR='/rex server/Current/bin' "${NU_BIN:-nu}" --no-config-file -c 'source config/nushell/config.nu; if ($env.PATH | first) != $env.REX_BIN_DIR { error make {msg: "Rex PATH precedence lost"} }'
if [[ -d /Applications/Rex\ Beta.app || -d /Applications/Rex.app ]]; then
  sandbox=$(mktemp -d "${TMPDIR:-/tmp}/rex-prefs-test.XXXXXX")
  trap 'rm -rf "$sandbox"' EXIT
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
  exit "${REX_IMPORT_EXIT:-0}"
fi
STUB
  chmod +x "$home/.local/bin/rex"
  HOME="$home" bash scripts/setup-rex.sh
  grep -Fx "write com.superlogical.rex Workspace.terminalStartupCommand -string \"$home/.local/bin/dots-tools\" desktop nu" "$log"
  grep -Fx 'write com.superlogical.rex Workspace.terminalFontPointSize -float 15' "$log"
  grep -Fx 'write com.superlogical.rex Workspace.terminalCellHeightPercent -float 12' "$log"
  grep -Fx "do -C client:test client.theme.import path=$home/.config/rex/ayu-dark" "$REX_CALLS"
  HOME="$home" REX_IMPORT_EXIT=1 bash scripts/setup-rex.sh | grep -F 'Ayu import pending:'
  if grep -E 'remoteControl|Tailscale|automatic-restart' "$log"; then exit 1; fi
else
  printf '%s\n' 'SKIP Rex preference writes: macOS Rex app not installed'
fi
printf '%s\n' 'PASS Rex shell and preferences contract'
