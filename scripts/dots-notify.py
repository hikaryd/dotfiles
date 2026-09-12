#!/usr/bin/env python3
"""Уведомление Ghostty из скрытой tmux-панели, без passthrough=all/daemon."""

import argparse
import json
import os
import re
import stat
import subprocess
import sys
import time
from pathlib import Path


def capture(argv):
    try:
        result = subprocess.run(
            argv, capture_output=True, text=True, timeout=3, check=False
        )
        return result.stdout.strip() if result.returncode == 0 else ""
    except (OSError, subprocess.TimeoutExpired):
        return ""


def frontmost_is_ghostty():
    # NSWorkspace: не требует Accessibility, не активирует приложения.
    app = capture(
        [
            "/usr/bin/osascript",
            "-l",
            "JavaScript",
            "-e",
            'ObjC.import("AppKit"); $.NSWorkspace.sharedWorkspace.frontmostApplication.bundleIdentifier.js',
        ]
    )
    return app == "com.mitchellh.ghostty"


def safe_label(value):
    return re.sub(r"[\x00-\x1f\x7f-\x9f;]", "", value)[:64]


def notification(status, seconds, project):
    title = "dots · OK" if status == 0 else f"dots · ошибка {status}"
    body = (
        f"{safe_label(project)} · {seconds:.0f}s"
        if project
        else f"Команда завершена · {seconds:.0f}s"
    )
    return f"\033]777;notify;{title};{body}\a".encode()


def deliver(payload, pane, force=False):
    if not os.environ.get("TMUX"):
        if not force and frontmost_is_ghostty():
            return "focused"
        # Hook stdout intentionally goes to /dev/null. /dev/tty is the
        # controlling terminal, independent of stdout/stderr redirections.
        try:
            fd = os.open(
                "/dev/tty", os.O_WRONLY | os.O_NOCTTY | os.O_NONBLOCK | os.O_NOFOLLOW
            )
            try:
                if not os.isatty(fd):
                    return "no-terminal"
                os.write(fd, payload)
                return "sent"
            finally:
                os.close(fd)
        except OSError:
            return "no-terminal"
    # tmux passthrough=on drops hidden-pane messages; all broadens access for
    # every program. Вместо этого доставляем только фиксированный OSC 777 в
    # принадлежащий текущему пользователю tty подключённого Ghostty-клиента.
    rows = capture(
        ["tmux", "list-clients", "-F", "#{client_tty}\t#{client_termname}\t#{pane_id}"]
    )
    clients = [line.split("\t") for line in rows.splitlines()]
    clients = [row for row in clients if len(row) == 3 and row[1] == "xterm-ghostty"]
    if not clients:
        return "no-ghostty-client"
    if not force and any(row[2] == pane for row in clients) and frontmost_is_ghostty():
        return "focused"
    for tty, _, _ in clients:
        if not re.fullmatch(r"/dev/tty[s0-9]+", tty):
            continue
        try:
            fd = os.open(tty, os.O_WRONLY | os.O_NOCTTY | os.O_NONBLOCK | os.O_NOFOLLOW)
            try:
                info = os.fstat(fd)
                if not stat.S_ISCHR(info.st_mode) or info.st_uid != os.getuid():
                    continue
                os.write(fd, payload)
                return "sent"
            finally:
                os.close(fd)
        except OSError:
            continue
    return "no-writable-client"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--test", action="store_true", help="один тест, даже если окно активно"
    )
    parser.add_argument("--status", type=int, default=0)
    parser.add_argument("--seconds", type=float, default=0)
    parser.add_argument("--pane", default=os.environ.get("TMUX_PANE", ""))
    args = parser.parse_args()
    if not 0 <= args.status <= 255 or not 0 <= args.seconds < 10**9:
        parser.error("status/seconds вне диапазона")
    if not args.test and args.seconds < 30:
        return 0
    config = Path(os.environ.get("XDG_CONFIG_HOME", Path.home() / ".config")) / "dots"
    try:
        presentation = (config / "style").read_text().strip() == "presentation"
    except OSError:
        presentation = False
    project = "" if presentation else Path.cwd().name
    result = deliver(
        notification(args.status, args.seconds, project), args.pane, args.test
    )
    # Только метаданные доставки, никаких команд, argv или истории.
    state = (
        Path(os.environ.get("XDG_STATE_HOME", Path.home() / ".local/state")) / "dots"
    )
    state.mkdir(parents=True, exist_ok=True, mode=0o700)
    target = state / "last-notification.json"
    fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "w") as stream:
        os.fchmod(stream.fileno(), 0o600)
        json.dump(
            {
                "result": result,
                "exit_code": args.status,
                "seconds": args.seconds,
                "at": int(time.time()),
            },
            stream,
        )
    if args.test:
        print(
            f"notification transport: {result}; отображение зависит от разрешений macOS/Focus"
        )
    return 0 if result in {"sent", "focused"} else 1


if __name__ == "__main__":
    sys.exit(main())
