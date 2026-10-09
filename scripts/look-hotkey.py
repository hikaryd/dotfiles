#!/usr/bin/env python3
"""Set Look's hotkey without replacing its other user settings."""

from pathlib import Path
import re


config = Path.home() / ".look" / "config"
config.parent.mkdir(parents=True, exist_ok=True)

lines = config.read_text().splitlines(keepends=True) if config.exists() else []
hotkey_line = "launcher_hotkey=alt+a\n"
matches = [i for i, line in enumerate(lines) if re.match(r"^\s*launcher_hotkey\s*=", line)]

if matches:
    lines[matches[0]] = hotkey_line
    for i in reversed(matches[1:]):
        del lines[i]
else:
    if lines and not lines[-1].endswith("\n"):
        lines[-1] += "\n"
    lines.append(hotkey_line)

updated = "".join(lines)
if not config.exists() or config.read_text() != updated:
    config.write_text(updated)
