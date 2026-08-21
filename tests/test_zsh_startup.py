#!/usr/bin/env python3

"""Regression check for first-prompt shell integrations."""

from __future__ import annotations

import os
import pty
import select
import shutil
import signal
import tempfile
import time
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[1]
PROMPT_READY = b"\x1b[?2004h"
HISTORY_PREFIX = b"echo FIRSTSHELL_"
HISTORY_SUFFIX = b"AUTOSUGGEST_MARKER"


def read_until(fd: int, marker: bytes, timeout: float) -> bytes:
    output = bytearray()
    deadline = time.monotonic() + timeout
    while marker not in output and time.monotonic() < deadline:
        ready, _, _ = select.select([fd], [], [], 0.1)
        if not ready:
            continue
        try:
            output.extend(os.read(fd, 65536))
        except OSError:
            break
    return bytes(output)


def main() -> None:
    zsh = shutil.which("zsh")
    if not zsh:
        raise SystemExit("zsh is not installed")

    with tempfile.TemporaryDirectory(prefix="dots-zsh-home-") as temporary_home:
        home = Path(temporary_home)
        (home / "dots").symlink_to(REPO_ROOT, target_is_directory=True)
        (home / ".cargo").mkdir()
        (home / ".cargo" / "env").touch()
        (home / ".zsh_history").write_text(": 1:0;echo FIRSTSHELL_AUTOSUGGEST_MARKER\n")

        environment = os.environ | {
            "HOME": str(home),
            "ZDOTDIR": str(REPO_ROOT / "config/zsh"),
            "TERM": "xterm-256color",
        }
        child_pid, terminal_fd = pty.fork()
        if child_pid == 0:
            os.chdir(REPO_ROOT)
            os.execve(zsh, [zsh, "-d", "-i"], environment)

        try:
            startup = read_until(terminal_fd, PROMPT_READY, 10)
            if PROMPT_READY not in startup:
                raise AssertionError(f"first prompt did not become editable:\n{startup!r}")

            os.write(terminal_fd, HISTORY_PREFIX)
            first_line = read_until(terminal_fd, HISTORY_SUFFIX, 2)
            if HISTORY_SUFFIX not in first_line:
                raise AssertionError(
                    "history autosuggestion was absent on the first prompt:\n"
                    f"{first_line!r}"
                )
        finally:
            try:
                os.write(terminal_fd, b"\x03exit\r")
            except OSError:
                pass
            os.close(terminal_fd)
            try:
                os.kill(child_pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            deadline = time.monotonic() + 2
            while time.monotonic() < deadline:
                waited_pid, _ = os.waitpid(child_pid, os.WNOHANG)
                if waited_pid == child_pid:
                    break
                time.sleep(0.05)
            else:
                os.kill(child_pid, signal.SIGKILL)
                os.waitpid(child_pid, 0)
            # .zcompdump compilation is intentionally detached by .zshrc.
            time.sleep(0.3)

    print("ok: first-prompt autosuggestions")


if __name__ == "__main__":
    main()
