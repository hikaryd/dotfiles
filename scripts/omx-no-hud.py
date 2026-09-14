#!/usr/bin/env python3
"""Disable HUD restoration through OMX's pinned plugin-hook launcher config.

Run again after `omx setup` or plugin resync. No package code or other hooks are
changed. Existing HUD panes must be retired separately after verifying ownership.
"""

import argparse
import json
import os
import tempfile
from datetime import datetime, timezone
from pathlib import Path


def without_hud(config: object) -> dict:
    if not isinstance(config, dict):
        raise TypeError("launcher must be an object")
    command = config.get("command")
    args = config.get("argsPrefix", [])
    if not isinstance(command, str) or not command.strip():
        raise ValueError("launcher command must be a non-empty string")
    if not isinstance(args, list) or not all(isinstance(arg, str) for arg in args):
        raise ValueError("launcher argsPrefix must be a list of strings")
    if command == "/usr/bin/env" and args[:1] == ["OMX_TMUX_HUD_OWNER=0"]:
        return dict(config)
    return dict(
        config,
        command="/usr/bin/env",
        argsPrefix=["OMX_TMUX_HUD_OWNER=0", command, *args],
    )


def apply(path: Path) -> bool:
    original = path.read_bytes()
    config = json.loads(original)
    updated = without_hud(config)
    if updated == config:
        print(f"Already HUD-free: {path}")
        return False
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
    backup = path.with_name(f"{path.name}.before-no-hud-{stamp}")
    with backup.open("xb") as stream:
        os.chmod(backup, 0o600)
        stream.write(original)
    with tempfile.NamedTemporaryFile(dir=path.parent, delete=False) as stream:
        temporary = Path(stream.name)
        stream.write((json.dumps(updated, indent=2) + "\n").encode())
    try:
        if path.read_bytes() != original:
            raise RuntimeError(f"Concurrent launcher change: {path}")
        os.replace(temporary, path)
    finally:
        temporary.unlink(missing_ok=True)
    print(f"Disabled HUD restore: {path}; backup: {backup}")
    return True


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "paths", nargs="*", type=Path, help="explicit pinned launcher JSON paths"
    )
    args = parser.parse_args()
    codex_home = Path(os.environ.get("CODEX_HOME", str(Path.home() / ".codex")))
    paths = args.paths or sorted(
        (codex_home / "plugins/cache/oh-my-codex-local/oh-my-codex").glob(
            "*/hooks/omx-command.json"
        )
    )
    if not paths:
        parser.error("No OMX pinned launchers found; supply their paths explicitly")
    for path in paths:
        apply(path)


if __name__ == "__main__":
    main()
