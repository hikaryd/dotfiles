#!/usr/bin/env python3
"""Optional real-binary regression: python3 tests/test-tuios-prefix.py [binary].

Starts only a disposable daemon and PTY. Never attaches to a user's session.
The default binary is the installed, pinned pure-Go release, not scripts/tuios.
"""
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import shlex
import shutil
import struct
import subprocess
import sys
import termios
import time
import tty

if len(sys.argv) > 1 and sys.argv[1] == "--record":
    tty.setraw(0)
    os.write(1, b"INPUT-RECORDER-READY\r\n")
    with open(sys.argv[2], "ab", buffering=0) as output:
        while data := os.read(0, 65536):
            output.write(data)
    sys.exit(0)

REPO = Path(__file__).resolve().parents[1]
BINARY = Path(sys.argv[1] if len(sys.argv) > 1 else Path.home() / ".local/share/dots/tuios/0.11.0/tuios").resolve()
with BINARY.open("rb") as binary_file:
    assert binary_file.read(2) != b"#!", "pass the real binary, not a wrapper that could redirect to the daily profile"
BASE = Path(os.environ.get("JCODE_SCRATCH_DIR", str(Path.home() / ".cache")))
ROOT = BASE / ("pfx-" + str(os.getpid()))
ROOT.mkdir(mode=0o700, parents=True)
for directory in ("config", "r", "state"):
    (ROOT / directory).mkdir(mode=0o700)
shutil.copytree(REPO / "config/tuios", ROOT / "config/tuios")
RECEIVED = ROOT / "received.bin"
RECEIVED.touch()
recorder = ROOT / "recorder.sh"
recorder.write_text("#!/bin/sh\nexec " + " ".join(shlex.quote(value) for value in [sys.executable, str(Path(__file__).resolve()), "--record", str(RECEIVED)]) + "\n")
recorder.chmod(0o700)
config = ROOT / "config/tuios/config.toml"
text = config.read_text().replace('preferred_shell = ""', "preferred_shell = " + json.dumps(str(recorder)))
# The custom Ctrl+A c command must use this fixture's daemon too.
text = text.replace('exec "$HOME/.local/bin/tuios" tmux new-window', f'exec "{BINARY}" tmux new-window')
config.write_text(text)
ENV = {key: value for key, value in os.environ.items() if not key.startswith(("HERDR_", "TUIOS_", "TMUX", "DOTS_TUIOS_"))}
ENV.update(XDG_CONFIG_HOME=str(ROOT / "config"), XDG_STATE_HOME=str(ROOT / "state"), XDG_RUNTIME_DIR=str(ROOT / "r"), TUIOS_SERVER="p", SHELL=str(recorder), TERM="xterm-256color")
LOG = (ROOT / "daemon.log").open("wb")
DAEMON = subprocess.Popen([str(BINARY), "daemon", "--server", "p", "--no-restore"], env=ENV, stdin=subprocess.DEVNULL, stdout=LOG, stderr=subprocess.STDOUT, start_new_session=True)
CLIENT, MASTER = None, None
CAPTURE, RESULTS = bytearray(), []


def cli(*args):
    result = subprocess.run([str(BINARY), *args, "--server", "p"], env=ENV, capture_output=True, text=True, timeout=10)
    if result.returncode:
        raise RuntimeError(f"{args}: {result.stderr} {result.stdout}")
    return json.loads(result.stdout) if "--json" in args else result.stdout


def drain(delay=0.15):
    end = time.monotonic() + delay
    if MASTER is None:
        time.sleep(delay)
        return
    while time.monotonic() < end:
        if not select.select([MASTER], [], [], max(0, end - time.monotonic()))[0]:
            return
        try:
            CAPTURE.extend(os.read(MASTER, 1048576))
        except OSError:
            return


def wait_for(predicate, timeout=8):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        try:
            if result := predicate():
                return result
        except (RuntimeError, FileNotFoundError, KeyError):
            pass
        drain(0.05)
    raise AssertionError(f"timed out, see {ROOT}")


def keys(data):
    os.write(MASTER, data)
    drain(0.3)


def expect(label, sequence, expected):
    start = len(RECEIVED.read_bytes())
    for chunk in sequence if isinstance(sequence, list) else [sequence]:
        keys(chunk)
    drain()
    actual = RECEIVED.read_bytes()[start:]
    passed = actual == expected
    RESULTS.append({"case": label, "passed": passed, "received_hex": actual.hex(), "expected_hex": expected.hex()})
    print(("PASS " if passed else "FAIL ") + label + ": " + repr(actual), flush=True)
    return passed


try:
    wait_for(lambda: (cli("ls", "--json"), True)[1])
    cli("new", "check", "--detach", "--cwd", str(REPO))
    MASTER, slave = pty.openpty()
    fcntl.ioctl(MASTER, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 150, 0, 0))
    CLIENT = subprocess.Popen([str(BINARY), "attach", "check", "--server", "p"], env=ENV, stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
    os.close(slave)
    wait_for(lambda: cli("session-info", "-s", "check", "--json").get("tui_attached"))
    wait_for(lambda: b"INPUT-RECORDER-READY" in CAPTURE)
    if not expect("attach immediately types", b"Z", b"Z"):
        keys(b"i")  # Continue independent regression cases on the old profile.
    expect("double Ctrl+A sends one Ctrl+A", b"\x01\x01Z", b"\x01Z")
    expect("Ctrl+A Ctrl+C cancels", b"\x01\x03Z", b"Z")
    # Separate Esc from Z, otherwise a terminal reads the pair as Alt+Z.
    if not expect("Ctrl+A Esc cancels to terminal input", [b"\x01", b"\x1b", b"Z"], b"Z"):
        keys(b"i")
    expect("unbound prefix key reaches pane", b"\x01~Z", b"~Z")
    expect("ordinary Esc reaches application", [b"\x1b", b"Z"], b"\x1bZ")
    expect("zoom returns to typing", b"\x01mZ", b"Z")
    keys(b"\x01|")
    wait_for(lambda: cli("session-info", "-s", "check", "--json")["window_count"] == 2)
    drain(0.6)
    expect("split returns to typing", b"Z", b"Z")
    expect("focus returns to typing", b"\x08Z", b"Z")
    expect("one-shot resize does not swallow hjl text", b"\x01hhjlZ", b"hjlZ")
    expect("sticky resize Esc returns to typing", [b"\x01Rhh", b"\x1b", b"Z"], b"Z")
    keys(b"\x01c")
    wait_for(lambda: cli("session-info", "-s", "check", "--json")["current_workspace"] == 2)
    drain(0.6)
    expect("new workspace returns to typing", b"Z", b"Z")
    expect("previous workspace returns to typing", b"\x01bZ", b"Z")
finally:
    for process in (CLIENT, DAEMON):
        if process is not None and process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
    if MASTER is not None:
        os.close(MASTER)
    (ROOT / "client.ansi").write_bytes(CAPTURE)
    (ROOT / "results.json").write_text(json.dumps(RESULTS, indent=2))
    LOG.close()
    print("Evidence: " + str(ROOT), flush=True)
assert RESULTS and all(row["passed"] for row in RESULTS), "prefix input regression failed"
