#!/usr/bin/env python3
"""Show the current Herdr workspace in the top tab bar."""

import json
import os
import subprocess
import sys
from pathlib import Path


def clean(value):
    return "".join(c for c in value if c.isprintable())[:22]


def main():
    style = (
        Path(os.environ.get("XDG_CONFIG_HOME", str(Path.home() / ".config")))
        / "dots/style"
    )
    try:
        if style.read_text().strip() == "presentation":
            print("PRESENTATION")
            return 0
    except FileNotFoundError:
        pass
    workspace_id = os.environ.get("HERDR_ACTIVE_WORKSPACE_ID", "")
    if not workspace_id:
        return 0
    result = subprocess.run(
        ["herdr", "workspace", "get", workspace_id],
        text=True,
        capture_output=True,
        timeout=2,
        check=False,
    )
    if result.returncode:
        return 1
    workspace = json.loads(result.stdout).get("result", {}).get("workspace", {})
    label = workspace.get("label", "")
    if isinstance(label, str) and clean(label):
        print(" " + clean(label))
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, ValueError, json.JSONDecodeError, subprocess.TimeoutExpired):
        sys.exit(1)
