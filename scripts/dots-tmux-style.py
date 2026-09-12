#!/usr/bin/env python3
"""Обратимые status overrides только у уже работающего tmux server."""

import json
import os
import subprocess
import sys
from pathlib import Path

PRESENTATION = {
    "status-left": "#[fg=#bfbdb6] PRESENTATION ",
    "window-status-format": " #I terminal ",
    "window-status-current-format": "#[fg=#73d0ff,bold] #I terminal ",
}
SNAPSHOT = "@dots-style-snapshot"


def tmux(*args):
    return subprocess.run(
        ["tmux", *args], capture_output=True, text=True, check=False, timeout=4
    )


def show(key):
    result = tmux("show-options", "-gqv", key)
    if result.returncode:
        raise RuntimeError(result.stderr.strip())
    return result.stdout.rstrip("\n")


def set_option(key, value):
    result = tmux("set-option", "-g", key, value)
    if result.returncode:
        raise RuntimeError(result.stderr.strip())


def apply(style):
    try:
        if tmux("list-sessions").returncode:
            return
    except FileNotFoundError:
        return
    raw = show(SNAPSHOT)
    saved = json.loads(raw) if raw else {}
    if style == "presentation":
        if not saved:
            saved = {key: show(key) for key in PRESENTATION}
            set_option(SNAPSHOT, json.dumps(saved))
        for key, value in PRESENTATION.items():
            set_option(key, value)
    elif saved:
        for key, previous in saved.items():
            # Не затираем пользовательскую правку, сделанную уже после профиля.
            if key in PRESENTATION and show(key) == PRESENTATION[key]:
                set_option(key, previous)
        result = tmux("set-option", "-gu", SNAPSHOT)
        if result.returncode:
            raise RuntimeError(result.stderr.strip())


if __name__ == "__main__":
    if sys.argv[1:] == ["--current"]:
        state = (
            Path(os.environ.get("XDG_CONFIG_HOME", Path.home() / ".config"))
            / "dots/style"
        )
        try:
            sys.argv[1] = state.read_text().strip()
        except FileNotFoundError:
            sys.argv[1] = "glass"
    if len(sys.argv) != 2 or sys.argv[1] not in {"glass", "focus", "presentation"}:
        sys.exit("Usage: dots-tmux-style.py glass|focus|presentation")
    try:
        apply(sys.argv[1])
    except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
        sys.exit(f"tmux style: {error}")
