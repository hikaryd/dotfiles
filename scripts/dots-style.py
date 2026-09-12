#!/usr/bin/env python3
"""Локальные обратимые профили; glass наследует основной конфиг без overrides."""

import argparse
import os
import subprocess
import tempfile
from pathlib import Path

MODES = ("glass", "focus", "presentation")


def atomic_write(path, content):
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    fd, temporary = tempfile.mkstemp(prefix="." + path.name, dir=path.parent)
    try:
        with os.fdopen(fd, "w") as stream:
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=(*MODES, "status"))
    args = parser.parse_args()
    config = Path(os.environ.get("XDG_CONFIG_HOME") or Path.home() / ".config") / "dots"
    state = config / "style"
    if args.mode == "status":
        current = state.read_text().strip() if state.exists() else "glass"
        print(current if current in MODES else "glass")
        return 0
    override = "# Managed by dots style; glass inherits the base configuration.\n"
    if args.mode != "glass":
        override += "background-opacity = 1\nbackground-blur = 0\ncustom-shader =\ncustom-shader-animation = false\n"
    if args.mode == "presentation":
        override += "font-size = 20\ntitle = Presentation\nforeground = f5f5f5\nselection-background = 405366\nselection-foreground = ffffff\n"
    atomic_write(config / "ghostty-style.conf", override)
    atomic_write(state, args.mode + "\n")
    helper = Path(__file__).resolve().with_name("dots-tmux-style.py")
    result = 0
    if helper.is_file():
        result = subprocess.run(
            ["python3", str(helper), args.mode], check=False
        ).returncode
    reload_helper = Path(__file__).resolve().with_name("dots-ghostty-reload.py")
    if reload_helper.is_file():
        reload_result = subprocess.run(
            ["python3", str(reload_helper)], check=False
        ).returncode
        result = result or reload_result
    else:
        print("Ghostty: Reload Config (Cmd+Shift+,); Reset Font Size (Cmd+0).")
    print(f"{args.mode}: saved.")
    print(
        "Neovim: applies on FocusGained. Presentation does not hide buffer or scrollback contents."
    )
    return result


if __name__ == "__main__":
    raise SystemExit(main())
