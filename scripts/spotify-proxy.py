#!/usr/bin/env python3
"""Configure Spotify's native HTTP proxy without changing other preferences."""

import argparse
import os
from pathlib import Path
import re
import sys
import tempfile


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=10810)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    if not 1 <= args.port <= 65535:
        parser.error("proxy port must be between 1 and 65535")

    prefs = Path.home() / "Library/Application Support/Spotify/prefs"
    values = {
        "network.proxy.mode": "2",
        "network.proxy.addr": f'"127.0.0.1:{args.port}@http"',
    }
    text = prefs.read_text() if prefs.exists() else ""
    configured = all(
        re.findall(rf"^{re.escape(key)}=(.*)$", text, re.MULTILINE) == [value]
        for key, value in values.items()
    )
    if args.check:
        raise SystemExit(0 if configured else 1)
    if configured:
        return

    lines = [
        line for line in text.splitlines()
        if line.partition("=")[0] not in values
    ]
    lines.extend(f"{key}={value}" for key, value in values.items())
    prefs.parent.mkdir(parents=True, exist_ok=True)
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(
            mode="w", dir=prefs.parent, prefix=".prefs-", delete=False
        ) as stream:
            temporary = Path(stream.name)
            stream.write("\n".join(lines) + "\n")
            stream.flush()
            os.fsync(stream.fileno())
        temporary.replace(prefs)
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)


if __name__ == "__main__":
    try:
        main()
    except (OSError, UnicodeError) as exc:
        print(
            f"spotify-proxy: could not access preferences ({type(exc).__name__})",
            file=sys.stderr,
        )
        raise SystemExit(2)
