#!/usr/bin/env python3
"""Run a private sing-box gateway as Android shell; supervise it from Termux.

The existing Hermes endpoint and device identity are read, never repaired here.
Only install copies config/binary; a live unmanaged trial is never terminated.
Stopping the Termux watchdog does not itself stop the Android shell service.
"""

from __future__ import annotations

import argparse
import contextlib
import fcntl
import json
import os
import re
import shlex
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import threading
import time
import urllib.request
from pathlib import Path

REMOTE = "/data/local/tmp/pixel-proxy"
SELF = Path(__file__).resolve()
JOURNAL_LIMIT = 262144

# Android's /system/bin/sh and toybox are sufficient; Termux binaries are not
# readable by UID 2000. No credentials are interpolated into this script.
SUPERVISOR = r'''#!/system/bin/sh
set -eu
umask 077
D=/data/local/tmp/pixel-proxy
SELF=$D/managed-supervisor.sh
BIN=$D/sing-box
CONFIG=$D/config.json
LOCK=$D/managed-run.lock
PID=$LOCK/pid
CHILD=$D/managed-child.pid
LOG=$D/managed-service.log

fail() { printf '%s\n' "pixel-proxy: $1" >&2; exit 1; }
stamp() {
    [ -r "/proc/$1/stat" ] || return 1
    IFS= read -r info < "/proc/$1/stat" || return 1
    rest=${info##*) }
    # shellcheck disable=SC2086 # Split trusted numeric stat fields; globbing is off.
    (set -f; set -- $rest; [ "$#" -ge 20 ] || exit 1; shift 19; printf '%s\n' "$1")
}
owned() {
    pid=$1; start=$2; kind=$3
    case "$pid:$start" in *[!0-9:]*|:*|*:) return 1;; esac
    [ "$pid" -gt 1 ] || return 1
    [ -r "/proc/$pid/cmdline" ] || return 1
    [ "$(stat -c %u "/proc/$pid" 2>/dev/null)" = 2000 ] || return 1
    [ "$(stamp "$pid")" = "$start" ] || return 1
    actual=$(tr '\000' '\n' < "/proc/$pid/cmdline")
    if [ "$kind" = supervisor ]; then
        expected=$(printf '%s\n' /system/bin/sh "$SELF" run)
    else
        expected=$(printf '%s\n' "$BIN" run -c "$CONFIG")
        [ "$(readlink "/proc/$pid/exe" 2>/dev/null)" = "$BIN" ] || return 1
    fi
    [ "$actual" = "$expected" ]
}
state_pid() {
    [ -f "$1" ] && [ ! -L "$1" ] || return 1
    IFS=' ' read -r pid start extra < "$1" || return 1
    [ -z "$extra" ] || return 1
    owned "$pid" "$start" "$2" || return 1
    printf '%s\n' "$pid"
}
write_state() {
    start=$(stamp "$2") || fail 'cannot establish process identity'
    printf '%s %s\n' "$2" "$start" > "$1.new"
    chmod 600 "$1.new"
    mv -f "$1.new" "$1"
}
log() {
    entry="$(date -u '+%Y-%m-%dT%H:%M:%SZ') $*"
    size=0
    if [ -f "$LOG" ]; then size=$(wc -c < "$LOG"); fi
    if [ "$((size + ${#entry} + 1))" -gt 262144 ]; then mv -f "$LOG" "$LOG.1"; fi
    printf '%s\n' "$entry" >> "$LOG"
    chmod 600 "$LOG"
}
terminate() {
    state=$1; type=$2
    [ -f "$state" ] && [ ! -L "$state" ] || return 0
    IFS=' ' read -r target target_start extra < "$state" || return 0
    [ -z "$extra" ] || return 0
    owned "$target" "$target_start" "$type" || return 0
    if ! kill -TERM "$target"; then
        owned "$target" "$target_start" "$type" && return 1
        return 0
    fi
    count=0
    while [ "$count" -lt 50 ]; do
        owned "$target" "$target_start" "$type" || return 0
        sleep 0.1; count=$((count + 1))
    done
    if owned "$target" "$target_start" "$type"; then kill -KILL "$target" || return 1; fi
    count=0
    while [ "$count" -lt 10 ]; do
        owned "$target" "$target_start" "$type" || return 0
        sleep 0.1; count=$((count + 1))
    done
    return 1
}
validate_layout() {
    [ "$(id -u)" = 2000 ] || fail 'Android shell UID 2000 is required'
    [ -d "$D" ] && [ ! -L "$D" ] || fail 'private runtime directory missing'
    [ "$(stat -c %u "$D")" = 2000 ] && [ "$(stat -c %a "$D")" = 700 ] || fail 'runtime directory ownership/mode invalid'
    for file in "$CONFIG" "$BIN" "$SELF"; do
        [ -f "$file" ] && [ ! -L "$file" ] && [ "$(stat -c %u "$file")" = 2000 ] || fail 'private runtime file ownership invalid'
    done
    [ "$(stat -c %a "$CONFIG")" = 600 ] || fail 'config must have mode 600'
    [ "$(stat -c %a "$BIN")" = 700 ] || fail 'binary must have mode 700'
    [ "$(stat -c %a "$SELF")" = 700 ] || fail 'supervisor must have mode 700'
    for file in "$LOCK" "$CHILD" "$LOG" "$LOG.1" "$CHILD.new"; do
        [ ! -L "$file" ] || fail 'managed state path is a symlink'
    done
}
status_service() {
    supervisor=$(state_pid "$PID" supervisor) || supervisor=''
    child=$(state_pid "$CHILD" child) || child=''
    if [ -n "$supervisor" ] && [ -n "$child" ]; then
        printf '{"state":"running","supervisor":%s,"child":%s}\n' "$supervisor" "$child"
    elif [ -n "$supervisor" ]; then
        printf '{"state":"restarting","supervisor":%s}\n' "$supervisor"
    elif [ -n "$child" ]; then
        printf '{"state":"orphaned","child":%s}\n' "$child"
    else printf '{"state":"stopped"}\n'; fi
}
run_service() {
    if ! mkdir "$LOCK" 2>/dev/null; then
        if state_pid "$PID" supervisor >/dev/null; then fail 'supervisor already running'; fi
        # A lock without a pid can belong to an initializing supervisor.
        if [ ! -f "$PID" ]; then
            age=$(( $(date +%s) - $(stat -c %Y "$LOCK") ))
            [ "$age" -gt 10 ] || fail 'supervisor lock is initializing'
        else rm -f "$PID"; fi
        rmdir "$LOCK" || fail 'stale lock contains unexpected files'
        mkdir "$LOCK" || fail 'supervisor lock is busy'
    fi
    chmod 700 "$LOCK"
    terminate "$CHILD" child || fail 'owned orphan stop failed'
    own_start=$(stamp "$$")
    write_state "$PID" "$$"
    stopping=0; child_pid=''; delay=2
    # shellcheck disable=SC2329 # Invoked by the EXIT trap.
    cleanup() {
        rc=$?
        trap - EXIT TERM INT
        if ! terminate "$CHILD" child; then
            log 'child_cleanup_failed'
            if [ "$rc" -eq 0 ]; then rc=1; fi
        fi
        if [ -n "$child_pid" ]; then wait "$child_pid" 2>/dev/null || :; fi
        if [ -f "$PID" ]; then
            IFS=' ' read -r recorded_pid recorded_start < "$PID" || recorded_pid=''
            if [ "$recorded_pid" = "$$" ] && [ "$recorded_start" = "$own_start" ]; then
                rm -f "$PID" "$CHILD"
                rmdir "$LOCK" || rc=1
            fi
        fi
        log "supervisor_exit status=$rc"
        exit "$rc"
    }
    # shellcheck disable=SC2329 # Invoked by TERM/INT traps.
    request_stop() { stopping=1; terminate "$CHILD" child || log 'child_stop_failed'; }
    trap cleanup EXIT
    trap request_stop TERM INT
    cd "$D"
    log 'supervisor_started'
    while [ "$stopping" -eq 0 ]; do
        launched=$(date +%s)
        "$BIN" run -c "$CONFIG" >/dev/null 2>&1 &
        child_pid=$!
        write_state "$CHILD" "$child_pid"
        log 'child_started'
        result=0
        wait "$child_pid" || result=$?
        rm -f "$CHILD"
        log "child_exit status=$result"
        child_pid=''
        [ "$stopping" -eq 0 ] || break
        elapsed=$(( $(date +%s) - launched ))
        if [ "$elapsed" -ge 60 ]; then delay=2; fi
        log "restart_delay seconds=$delay"
        pause=0
        while [ "$pause" -lt "$((delay * 2))" ] && [ "$stopping" -eq 0 ]; do
            sleep 0.5; pause=$((pause + 1))
        done
        if [ "$delay" -lt 30 ]; then
            delay=$((delay * 2))
            if [ "$delay" -gt 30 ]; then delay=30; fi
        fi
    done
    exit 0
}
validate_layout
case "${1:-}" in
    run) run_service;;
    start)
        if state_pid "$PID" supervisor >/dev/null; then status_service; exit 0; fi
        (cd "$D" && "$BIN" check -c "$CONFIG" >/dev/null 2>&1) || fail 'config validation failed'
        nohup /system/bin/sh "$SELF" run >/dev/null 2>&1 </dev/null &
        count=0
        while [ "$count" -lt 30 ]; do
            if state_pid "$PID" supervisor >/dev/null; then status_service; exit 0; fi
            sleep 0.1; count=$((count + 1))
        done
        fail 'managed supervisor start was not confirmed';;
    stop)
        terminate "$PID" supervisor || fail 'owned supervisor stop failed'
        terminate "$CHILD" child || fail 'owned child stop failed'
        if state_pid "$PID" supervisor >/dev/null || state_pid "$CHILD" child >/dev/null; then fail 'managed process still alive'; fi
        rm -f "$PID" "$CHILD"
        if [ -d "$LOCK" ]; then rmdir "$LOCK" || fail 'managed lock cleanup failed'; fi
        status_service;;
    status) status_service;;
    *) fail 'expected start, stop, status, or run';;
esac
'''


class RuntimeFailure(RuntimeError):
    """A sanitized operation error; raw ADB/server output stays in memory."""


def private_file(path: Path) -> bytes:
    if path.is_symlink() or not path.is_file():
        raise RuntimeFailure("private local input missing or symlinked")
    info = path.stat()
    if info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o600:
        raise RuntimeFailure("private local input must be owned by this user with mode 600")
    if info.st_size > 2 * 1024 * 1024:
        raise RuntimeFailure("private local input exceeds size limit")
    return path.read_bytes()


def atomic_write(path: Path, payload: bytes, mode: int = 0o600) -> None:
    if path.is_symlink():
        raise RuntimeFailure("managed local output is a symlink")
    fd, temporary = tempfile.mkstemp(prefix=".pixel-proxy-", dir=path.parent)
    try:
        os.fchmod(fd, mode)
        with os.fdopen(fd, "wb") as stream:
            stream.write(payload)
        os.replace(temporary, path)
    finally:
        with contextlib.suppress(FileNotFoundError):
            os.unlink(temporary)


def shell_command(script: str) -> list[str]:
    """ADB joins shell arguments; quote once for the Android outer shell."""
    return ["shell", "-T", "/system/bin/sh", "-c", shlex.quote(script)]


class Gateway:
    def __init__(self, home: Path | None = None):
        self.home = home or Path.home()
        self.local = self.home / "pixel-proxy"
        self.bridge = self.home / "hermes-pixel"
        self._active_adb = None
        self.adb = shutil.which("adb")
        if not self.adb:
            raise RuntimeFailure("ADB is not installed")
        if self.local.is_symlink():
            raise RuntimeFailure("private local service directory is a symlink")
        self.local.mkdir(mode=0o700, parents=True, exist_ok=True)
        self.local.chmod(0o700)

    def _raw(self, endpoint: str, args: list[str], payload: bytes | None = None, timeout: int = 12) -> subprocess.CompletedProcess:
        try:
            command = [self.adb, "-s", endpoint, *args]
            # ADB can drain SSH/CLI stdin even for getprop. Give it input only
            # for our explicit private file transfer; every probe uses DEVNULL.
            with subprocess.Popen(
                command, stdin=subprocess.DEVNULL if payload is None else subprocess.PIPE,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            ) as process:
                self._active_adb = process
                try:
                    stdout, stderr = process.communicate(payload, timeout=timeout)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.communicate()
                    raise
                finally:
                    self._active_adb = None
                result = subprocess.CompletedProcess(command, process.returncode, stdout, stderr)
        except (OSError, subprocess.TimeoutExpired) as error:
            raise RuntimeFailure("ADB operation unavailable or timed out") from error
        if result.returncode != 0:
            raise RuntimeFailure(f"ADB operation failed with exit status {result.returncode}")
        return result

    def verify(self) -> str:
        try:
            identity = json.loads(private_file(self.bridge / "config.json"))
            serial = identity["serial"]
            endpoint = (self.bridge / "adb-endpoint").read_text().strip()
        except (OSError, ValueError, KeyError, TypeError) as error:
            raise RuntimeFailure("existing device identity or endpoint unavailable") from error
        if not isinstance(serial, str) or not serial.strip():
            raise RuntimeFailure("configured device identity is empty")
        if not re.fullmatch(r"127\.0\.0\.1:[1-9][0-9]{0,4}", endpoint):
            raise RuntimeFailure("existing endpoint must use IPv4 loopback")
        if int(endpoint.rsplit(":", 1)[1]) > 65535:
            raise RuntimeFailure("existing endpoint port is invalid")
        state = self._raw(endpoint, ["get-state"])
        if state.stdout.strip() != b"device":
            raise RuntimeFailure("existing ADB device is not ready")
        observed = self._raw(endpoint, shell_command("getprop ro.serialno"))
        if observed.stdout.decode(errors="replace").strip() != serial:
            raise RuntimeFailure("ADB device identity does not match configured Pixel")
        return endpoint

    def shell(self, script: str, payload: bytes | None = None, timeout: int = 12) -> bytes:
        # Each operation rereads and verifies the existing endpoint and serial.
        endpoint = self.verify()
        return self._raw(endpoint, shell_command(script), payload, timeout).stdout

    @contextlib.contextmanager
    def lock(self, name: str, nonblocking: bool = False):
        path = self.local / name
        flags = os.O_CREAT | os.O_RDWR | getattr(os, "O_NOFOLLOW", 0)
        fd = os.open(path, flags, 0o600)
        try:
            deadline = time.monotonic() + 10
            while True:
                try:
                    fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    break
                except BlockingIOError as error:
                    if nonblocking or time.monotonic() >= deadline:
                        raise RuntimeFailure("another managed lifecycle operation is running") from error
                    time.sleep(0.1)
            yield
        finally:
            os.close(fd)

    def log(self, event: str) -> None:
        # Callers supply fixed event names and integer status codes, never errors.
        if not re.fullmatch(r"[a-z_]+(?: [a-z_]+=[a-z_0-9]+)*", event):
            raise RuntimeFailure("unsafe lifecycle event")
        path = self.local / "adb-watchdog.log"
        previous = self.local / "adb-watchdog.log.1"
        if path.is_symlink() or previous.is_symlink():
            raise RuntimeFailure("journal path is a symlink")
        entry = (time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()) + " " + event + "\n").encode()
        if path.exists() and path.stat().st_size + len(entry) > JOURNAL_LIMIT:
            os.replace(path, previous)
        fd = os.open(path, os.O_CREAT | os.O_APPEND | os.O_WRONLY | getattr(os, "O_NOFOLLOW", 0), 0o600)
        with os.fdopen(fd, "ab") as stream:
            stream.write(entry)

    def install(self) -> dict:
        config = private_file(self.local / "config.json")
        try:
            json.loads(config)
        except ValueError as error:
            raise RuntimeFailure("local config.json is invalid JSON") from error
        source = shutil.which("sing-box")
        if not source:
            raise RuntimeFailure("Termux sing-box is not installed")
        binary_path = Path(source)
        if binary_path.stat().st_size > 256 * 1024 * 1024:
            raise RuntimeFailure("sing-box binary exceeds size limit")
        binary = binary_path.read_bytes()
        if binary[:4] != b"\x7fELF":
            raise RuntimeFailure("sing-box must be an Android executable ELF")
        # Refuse redeploy while *any* process uses this binary, including a trial.
        self.shell(f'''set -eu; umask 077
[ "$(id -u)" = 2000 ] || exit 10
[ ! -L {REMOTE} ] || exit 11
mkdir -p {REMOTE}; chmod 700 {REMOTE}
[ "$(stat -c %u {REMOTE})" = 2000 ] || exit 12
if [ -f {REMOTE}/managed-supervisor.sh ]; then
  current=$(/system/bin/sh {REMOTE}/managed-supervisor.sh status) || exit 13
  [ "$current" = '{{"state":"stopped"}}' ] || exit 13
fi
for p in /proc/[0-9]*/exe; do
  if [ "$(readlink "$p" 2>/dev/null)" = {REMOTE}/sing-box ]; then exit 14; fi
done
for name in sing-box config.json managed-supervisor.sh; do
  [ ! -L {REMOTE}/"$name" ] && [ ! -L {REMOTE}/"$name".new ] || exit 15
done
''')
        for name, payload, mode in [
            ("sing-box", binary, "700"),
            ("config.json", config, "600"),
            ("managed-supervisor.sh", SUPERVISOR.encode(), "700"),
        ]:
            self.shell(
                f"set -eu; umask 077; cat > {REMOTE}/{name}.new; "
                f"chmod {mode} {REMOTE}/{name}.new; mv -f {REMOTE}/{name}.new {REMOTE}/{name}",
                payload, timeout=90,
            )
        self.shell(f"cd {REMOTE}; ./sing-box check -c config.json >/dev/null 2>&1", timeout=20)
        self.log("runtime_installed")
        return {"state": "installed", "identity_verified": True}

    def remote_status(self) -> dict:
        output = self.shell(f"/system/bin/sh {REMOTE}/managed-supervisor.sh status")
        try:
            result = json.loads(output)
        except ValueError as error:
            raise RuntimeFailure("managed runtime status is invalid") from error
        if not isinstance(result, dict) or result.get("state") not in {"running", "restarting", "orphaned", "stopped"}:
            raise RuntimeFailure("managed runtime status is unexpected")
        for key in ("supervisor", "child"):
            if key in result and (type(result[key]) is not int or result[key] <= 1):
                raise RuntimeFailure("managed runtime process identity is invalid")
        return {k: v for k, v in result.items() if k in {"state", "supervisor", "child"}}

    def healthy(self) -> bool:
        try:
            config = json.loads(private_file(self.local / "config.json"))
            api = config["experimental"]["clash_api"]
            controller = api["external_controller"]
            if not re.fullmatch(r"127\.0\.0\.1:[1-9][0-9]{0,4}", controller):
                raise RuntimeFailure("health API must use IPv4 loopback")
            headers = {}
            secret = api.get("secret")
            if secret:
                headers["Authorization"] = "Bearer " + secret
            request = urllib.request.Request("http://" + controller + "/version", headers=headers)
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            with opener.open(request, timeout=3) as response:
                payload = response.read(4096)
                body = json.loads(payload)
                return response.status == 200 and isinstance(body, dict) and isinstance(body.get("version"), str) and bool(body["version"])
        except RuntimeFailure:
            raise
        except (OSError, ValueError, KeyError, TypeError):
            return False

    def status(self) -> dict:
        result = self.remote_status()
        result["healthy"] = result["state"] == "running" and self.healthy()
        return result

    def start(self) -> dict:
        self.shell(f"/system/bin/sh {REMOTE}/managed-supervisor.sh start", timeout=20)
        deadline = time.monotonic() + 15
        stable_child = None
        stable_since = time.monotonic()
        while time.monotonic() < deadline:
            state = self.remote_status()
            child = state.get("child") if state["state"] == "running" else None
            if child != stable_child:
                stable_child, stable_since = child, time.monotonic()
            if child and time.monotonic() - stable_since >= 2 and self.healthy():
                self.log("runtime_started")
                return {**state, "healthy": True}
            time.sleep(0.5)
        raise RuntimeFailure("managed runtime did not become stable and healthy")

    def stop(self) -> dict:
        self.stop_watchdog()
        self.shell(f"/system/bin/sh {REMOTE}/managed-supervisor.sh stop", timeout=25)
        result = self.remote_status()
        if result["state"] != "stopped":
            raise RuntimeFailure("managed runtime did not stop")
        self.log("runtime_stopped")
        return result

    def watchdog_identity(self) -> tuple[int, str] | None:
        try:
            stored = json.loads(private_file(self.local / "adb-watchdog.pid"))
            pid, stamp = stored["pid"], stored["start"]
            if type(pid) is not int or pid <= 1 or not isinstance(stamp, str):
                return None
            if Path(f"/proc/{pid}").stat().st_uid != os.getuid():
                return None
            if process_start(pid) != stamp:
                return None
            args = Path(f"/proc/{pid}/cmdline").read_bytes().split(b"\0")
            if args[1:3] != [os.fsencode(SELF), b"run"] or any(args[3:]):
                return None
            return pid, stamp
        except (OSError, ValueError, KeyError, RuntimeFailure):
            return None

    def stop_watchdog(self) -> None:
        identity = self.watchdog_identity()
        if not identity:
            return
        pid, _ = identity
        os.kill(pid, signal.SIGTERM)
        deadline = time.monotonic() + 16
        while self.watchdog_identity() == identity:
            if time.monotonic() >= deadline:
                if self.watchdog_identity() == identity:
                    os.kill(pid, signal.SIGKILL)
                break
            time.sleep(0.1)
        if self.watchdog_identity() == identity:
            raise RuntimeFailure("managed watchdog did not stop")

    def run(self) -> None:
        # Normalize argv for exact /proc process identity even when called relative.
        args = Path("/proc/self/cmdline").read_bytes().split(b"\0")
        if args[1:3] != [os.fsencode(SELF), b"run"]:
            os.execv(sys.executable, [sys.executable, str(SELF), "run"])
        stopping = threading.Event()
        def request_stop(*_):
            stopping.set()
            process = self._active_adb
            if process is not None and process.poll() is None:
                # This Popen child belongs to this watchdog; no external ADB
                # server, bridge process, or authenticated endpoint is changed.
                with contextlib.suppress(ProcessLookupError):
                    process.terminate()
        signal.signal(signal.SIGTERM, request_stop)
        signal.signal(signal.SIGINT, request_stop)
        with self.lock("adb-watchdog.lock", nonblocking=True):
            atomic_write(self.local / "adb-watchdog.pid", json.dumps({"pid": os.getpid(), "start": process_start(os.getpid())}).encode())
            self.log("watchdog_started")
            delay = 2
            try:
                while not stopping.is_set():
                    try:
                        with self.lock("adb-control.lock"):
                            status = self.status()
                            if status["state"] != "running" or not status["healthy"]:
                                self.start()
                        delay = 2
                        stopping.wait(10)
                    except (RuntimeFailure, OSError):
                        self.log("watchdog_retry delay=" + str(delay))
                        stopping.wait(delay)
                        delay = min(delay * 2, 30)
            finally:
                with contextlib.suppress(FileNotFoundError):
                    (self.local / "adb-watchdog.pid").unlink()
                self.log("watchdog_stopped")

    def install_boot(self) -> dict:
        boot_dir = self.home / ".termux" / "boot"
        if boot_dir.is_symlink():
            raise RuntimeFailure("boot directory is a symlink")
        boot_dir.mkdir(parents=True, mode=0o700, exist_ok=True)
        target = boot_dir / "30-pixel-proxy"
        marker = "# Managed by pixel-proxy-adb.py"
        if target.is_symlink():
            raise RuntimeFailure("managed boot file is a symlink")
        if target.exists():
            old = target.read_text()
            if marker not in old.splitlines() and "# Managed by pixel-proxy-service.sh" not in old.splitlines():
                raise RuntimeFailure("boot file already belongs to another service")
        prefix = os.environ.get("PREFIX", "/data/data/com.termux/files/usr")
        script = (
            "#!" + prefix + "/bin/sh\n" + marker + "\n"
            "termux-wake-lock >/dev/null 2>&1 || exit 1\n"
            "nohup " + shlex.quote(sys.executable) + " " + shlex.quote(str(SELF))
            + " run >/dev/null 2>&1 </dev/null &\n"
        )
        atomic_write(target, script.encode(), 0o700)
        self.log("boot_installed")
        return {"state": "boot_installed", "existing_boot_scripts_preserved": True}


def process_start(pid: int) -> str:
    fields = Path(f"/proc/{pid}/stat").read_text().rsplit(") ", 1)[1].split()
    value = fields[19]
    if not value.isdigit():
        raise RuntimeFailure("process start identity is invalid")
    return value


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("install", "start", "stop", "status", "run", "install-boot"))
    args = parser.parse_args()
    try:
        gateway = Gateway()
        if args.command == "run":
            gateway.run()
            return 0
        # Stop the watchdog before taking its control lock to avoid a deadlock.
        if args.command == "stop":
            gateway.stop_watchdog()
        with gateway.lock("adb-control.lock"):
            result = getattr(gateway, args.command.replace("-", "_"))()
        print(json.dumps(result, ensure_ascii=False))
        return 0 if args.command != "status" or (result["state"] == "running" and result["healthy"]) else 1
    except RuntimeFailure as error:
        print(json.dumps({"state": "failed", "operation": args.command, "error": str(error)}), file=sys.stderr)
        return 1
    except (OSError, ValueError):
        # Raw exceptions can contain response headers, config values, or paths.
        print(json.dumps({"state": "failed", "operation": args.command, "error": "operation failed; inspect protected lifecycle state and device readiness"}), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
