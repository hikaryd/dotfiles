#!/data/data/com.termux/files/usr/bin/bash
# Private Termux supervisor. It never releases the shared Termux wake lock.
set -euo pipefail
umask 077

SERVICE_DIR=${PIXEL_PROXY_DIR:-"$HOME/pixel-proxy"}
SERVICE_SCRIPT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)/$(basename -- "${BASH_SOURCE[0]}")
CONFIG="$SERVICE_DIR/config.json"
SUPERVISOR_STATE="$SERVICE_DIR/supervisor.pid"
CHILD_STATE="$SERVICE_DIR/sing-box.pid"
JOURNAL="$SERVICE_DIR/service.log"
JOURNAL_LIMIT=262144
OWNER_UID=$(id -u)
SING_BOX=$(command -v sing-box || true)

fail() { printf 'pixel-proxy: %s\n' "$*" >&2; exit 1; }

usage() {
  printf '%s\n' \
    'Usage: pixel-proxy-service.sh start|stop|status|run|install-boot' \
    'Config: ~/pixel-proxy/config.json (or PIXEL_PROXY_DIR/config.json).' \
    'Journal: lifecycle and exit codes only; at most two 256 KiB files.' \
    'start confirms the managed processes; check proxy connectivity separately.' \
    "stop is idempotent and does not release another service's wake lock."
}

ensure_layout() {
  [[ "$SERVICE_DIR" == /* && ! -L "$SERVICE_DIR" ]] || fail 'service directory must be an absolute, real directory'
  mkdir -p -- "$SERVICE_DIR"
  chmod 700 -- "$SERVICE_DIR"
  local name
  for name in supervisor.pid sing-box.pid service.log service.log.1 run.lock control.lock; do
    [[ ! -L "$SERVICE_DIR/$name" ]] || fail 'a service-owned state path is a symlink'
  done
  command -v flock >/dev/null || fail 'flock is required (Termux util-linux)'
}

validate_config() {
  [[ -n "$SING_BOX" && "$SING_BOX" == /* && -x "$SING_BOX" ]] || fail 'sing-box is not installed'
  [[ -f "$CONFIG" && ! -L "$CONFIG" ]] || fail 'private config.json is missing or is a symlink'
  [[ "$(stat -c %u -- "$CONFIG")" == "$OWNER_UID" ]] || fail 'config.json belongs to another user'
  chmod 600 -- "$CONFIG"
  if ! (cd -- "$SERVICE_DIR" && "$SING_BOX" check -c "$CONFIG" >/dev/null 2>&1); then
    fail 'sing-box config validation failed; no service was started'
  fi
}

process_start() {
  local info rest
  [[ -r "/proc/$1/stat" ]] || return 1
  IFS= read -r info < "/proc/$1/stat" || return 1
  rest=${info##*) }
  # /proc stat field 22; the parenthesized command may contain spaces.
  awk '{print $20}' <<< "$rest"
}

owned_process() {
  local pid=$1 stamp=$2 kind=$3 arg
  local -a args=()
  [[ "$pid" =~ ^[1-9][0-9]*$ && "$stamp" =~ ^[0-9]+$ ]] || return 1
  [[ -d "/proc/$pid" ]] || return 1
  [[ "$(stat -c %u -- "/proc/$pid" 2>/dev/null)" == "$OWNER_UID" ]] || return 1
  [[ "$(process_start "$pid")" == "$stamp" ]] || return 1
  [[ -r "/proc/$pid/cmdline" ]] || return 1
  while IFS= read -r -d '' arg; do args+=("$arg"); done < "/proc/$pid/cmdline"
  case "$kind" in
    supervisor)
      [[ ${#args[@]} -eq 3 && "${args[1]}" == "$SERVICE_SCRIPT" && "${args[2]}" == run ]]
      ;;
    child)
      [[ ${#args[@]} -eq 4 && "${args[0]}" == "$SING_BOX" && "${args[1]}" == run && "${args[2]}" == -c && "${args[3]}" == "$CONFIG" && "/proc/$pid/exe" -ef "$SING_BOX" ]]
      ;;
    *) return 1 ;;
  esac
}

state_pid() {
  local path=$1 kind=$2 pid stamp extra
  [[ -f "$path" && ! -L "$path" ]] || return 1
  read -r pid stamp extra < "$path" || return 1
  [[ -z "$extra" ]] || return 1
  owned_process "$pid" "$stamp" "$kind" || return 1
  printf '%s\n' "$pid"
}

write_state() {
  local path=$1 pid=$2 stamp temporary
  stamp=$(process_start "$pid") || fail 'cannot establish managed process identity'
  [[ "$stamp" =~ ^[0-9]+$ ]] || fail 'invalid managed process start time'
  temporary=$(mktemp "$SERVICE_DIR/.pid.XXXXXX")
  printf '%s %s\n' "$pid" "$stamp" > "$temporary"
  chmod 600 -- "$temporary"
  mv -f -- "$temporary" "$path"
}

journal() {
  local entry size=0
  entry="$(date -u '+%Y-%m-%dT%H:%M:%SZ') $*"
  if [[ -f "$JOURNAL" ]]; then size=$(wc -c < "$JOURNAL"); fi
  if (( size + ${#entry} + 1 > JOURNAL_LIMIT )); then
    mv -f -- "$JOURNAL" "$JOURNAL.1"
  fi
  printf '%s\n' "$entry" >> "$JOURNAL"
  chmod 600 -- "$JOURNAL"
}

terminate_owned() {
  local path=$1 kind=$2 pid stamp extra attempt
  [[ -f "$path" && ! -L "$path" ]] || return 0
  read -r pid stamp extra < "$path" || return 0
  [[ -z "$extra" ]] || return 0
  owned_process "$pid" "$stamp" "$kind" || return 0
  kill -TERM "$pid" || return 1
  for ((attempt=0; attempt<50; attempt++)); do
    owned_process "$pid" "$stamp" "$kind" || return 0
    sleep 0.1
  done
  if owned_process "$pid" "$stamp" "$kind"; then
    kill -KILL "$pid" || return 1
  fi
  for ((attempt=0; attempt<10; attempt++)); do
    owned_process "$pid" "$stamp" "$kind" || return 0
    sleep 0.1
  done
  return 1
}

status_service() {
  local supervisor child
  supervisor=$(state_pid "$SUPERVISOR_STATE" supervisor) || supervisor=''
  child=$(state_pid "$CHILD_STATE" child) || child=''
  if [[ -n "$supervisor" && -n "$child" ]]; then
    printf 'running supervisor=%s child=%s\n' "$supervisor" "$child"
    return 0
  elif [[ -n "$supervisor" ]]; then
    printf 'restarting supervisor=%s\n' "$supervisor"
    return 2
  elif [[ -n "$child" ]]; then
    printf 'orphaned child=%s\n' "$child"
    return 2
  fi
  printf 'stopped\n'
  return 1
}

start_service() {
  local supervisor child attempt
  exec 8> "$SERVICE_DIR/control.lock"
  flock -w 10 8 || fail 'another lifecycle operation is still running'
  supervisor=$(state_pid "$SUPERVISOR_STATE" supervisor) || supervisor=''
  if [[ -n "$supervisor" ]]; then
    status_service
    return
  fi
  validate_config
  terminate_owned "$CHILD_STATE" child || fail 'could not stop the owned orphan process'
  nohup "$BASH" "$SERVICE_SCRIPT" run 8>&- >/dev/null 2>&1 </dev/null &
  for ((attempt=0; attempt<50; attempt++)); do
    supervisor=$(state_pid "$SUPERVISOR_STATE" supervisor) || supervisor=''
    child=$(state_pid "$CHILD_STATE" child) || child=''
    if [[ -n "$supervisor" && -n "$child" ]]; then
      status_service
      return
    fi
    sleep 0.1
  done
  fail 'start did not confirm both managed processes; inspect the private lifecycle journal'
}

stop_service() {
  exec 8> "$SERVICE_DIR/control.lock"
  flock -w 10 8 || fail 'another lifecycle operation is still running'
  terminate_owned "$SUPERVISOR_STATE" supervisor || fail 'could not stop the owned supervisor'
  terminate_owned "$CHILD_STATE" child || fail 'could not stop the owned sing-box process'
  if state_pid "$SUPERVISOR_STATE" supervisor >/dev/null || state_pid "$CHILD_STATE" child >/dev/null; then
    fail 'a managed process is still alive'
  fi
  rm -f -- "$SUPERVISOR_STATE" "$CHILD_STATE"
  printf 'stopped\n'
}

run_service() {
  local stopping=0 child_pid='' delay=2 launched elapsed result stamp own_stamp pause
  exec 9> "$SERVICE_DIR/run.lock"
  flock -n 9 || fail 'the service supervisor is already locked'
  validate_config
  terminate_owned "$CHILD_STATE" child || fail 'could not stop the owned orphan process'
  own_stamp=$(process_start "$$")
  write_state "$SUPERVISOR_STATE" "$$"

  # shellcheck disable=SC2329 # Invoked by the EXIT trap while run locals are live.
  cleanup() {
    local result=$?
    trap - EXIT TERM INT
    if ! terminate_owned "$CHILD_STATE" child; then
      printf 'pixel-proxy: child cleanup failed\n' >&2
      if (( result == 0 )); then result=1; fi
    fi
    if [[ -n "$child_pid" ]]; then wait "$child_pid" 2>/dev/null || :; fi
    if [[ -f "$SUPERVISOR_STATE" ]]; then
      read -r child_pid stamp < "$SUPERVISOR_STATE" || child_pid=''
      if [[ "$child_pid" == "$$" && "$stamp" == "$own_stamp" ]]; then
        rm -f -- "$SUPERVISOR_STATE" "$CHILD_STATE"
      fi
    fi
    journal "supervisor_exit status=$result"
    exit "$result"
  }
  # shellcheck disable=SC2329 # Invoked by the TERM/INT traps.
  request_stop() {
    stopping=1
    terminate_owned "$CHILD_STATE" child || printf 'pixel-proxy: child stop failed\n' >&2
  }
  trap cleanup EXIT
  trap request_stop TERM INT
  if command -v termux-wake-lock >/dev/null; then
    if ! termux-wake-lock >/dev/null 2>&1; then
      journal 'wake_lock_failed'
      fail 'Termux wake lock request failed'
    fi
  fi
  cd -- "$SERVICE_DIR"
  journal 'supervisor_started'
  while (( ! stopping )); do
    launched=$SECONDS
    # Only safe lifecycle metadata reaches our journal. The config may hold keys.
    "$SING_BOX" run -c "$CONFIG" 9>&- >/dev/null 2>&1 &
    child_pid=$!
    write_state "$CHILD_STATE" "$child_pid"
    journal 'child_started'
    result=0
    wait "$child_pid" || result=$?
    rm -f -- "$CHILD_STATE"
    journal "child_exit status=$result"
    (( stopping )) && break
    elapsed=$(( SECONDS - launched ))
    if (( elapsed >= 60 )); then delay=2; fi
    journal "restart_delay seconds=$delay"
    child_pid=''
    pause=0
    while (( pause < delay * 2 && ! stopping )); do
      sleep 0.5
      pause=$(( pause + 1 ))
    done
    (( stopping )) && break
    if (( delay < 30 )); then
      delay=$(( delay * 2 ))
      if (( delay > 30 )); then delay=30; fi
    fi
  done
  exit 0
}

install_boot() {
  local boot_dir="$HOME/.termux/boot" boot temporary
  boot="$boot_dir/30-pixel-proxy"
  [[ ! -L "$boot_dir" && ! -L "$boot" ]] || fail 'boot path is a symlink'
  mkdir -p -- "$boot_dir"
  if [[ -e "$boot" ]] && ! grep -q '^# Managed by pixel-proxy-service.sh$' "$boot"; then
    fail '30-pixel-proxy already exists and is not owned by this installer'
  fi
  temporary=$(mktemp "$boot_dir/.pixel-proxy.XXXXXX")
  {
    printf '%s\n' '#!/data/data/com.termux/files/usr/bin/bash' '# Managed by pixel-proxy-service.sh'
    printf 'export PIXEL_PROXY_DIR=%q\n' "$SERVICE_DIR"
    printf 'exec %q %q start >/dev/null 2>&1\n' "$BASH" "$SERVICE_SCRIPT"
  } > "$temporary"
  chmod 700 -- "$temporary"
  mv -f -- "$temporary" "$boot"
  printf 'installed boot/30-pixel-proxy; existing boot scripts preserved\n'
}

[[ $# -eq 1 ]] || { usage >&2; exit 2; }
case "$1" in
  -h|--help|help) usage; exit 0 ;;
  start|stop|status|run|install-boot) ensure_layout ;;
  *) usage >&2; exit 2 ;;
esac
case "$1" in
  start) start_service ;;
  stop) stop_service ;;
  status) status_service ;;
  run) run_service ;;
  install-boot) install_boot ;;
esac
