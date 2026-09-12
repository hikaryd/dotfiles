#!/usr/bin/env python3
"""Read-only diagnostics for this dotfiles checkout; never source user config."""

import argparse
import json
import os
import re
import shutil
import stat
import subprocess
import sys
from pathlib import Path

try:
    import tomllib
except ImportError:
    tomllib = None

CRITICAL = {
    "~/.zshrc",
    "~/.zshenv",
    "~/.config/zsh/functions",
    "~/.config/tmux/tmux.conf",
    "~/.config/ghostty/config",
    "~/.config/nvim",
}


def managed_links(root):
    """Parse only this repository's block-form link/path subset, not arbitrary YAML."""
    for file in sorted((root / "steps").glob("*.yml")):
        in_links = False
        destination = None
        for line in file.read_text().splitlines():
            if line.startswith("- "):
                in_links = line.strip() == "- link:"
                destination = None
            if not in_links:
                continue
            match = re.fullmatch(r"    (~/.+):\s*", line)
            if match:
                destination = match[1]
            match = re.fullmatch(r"      path: (.+?)\s*", line)
            if match and destination:
                yield destination, root / match[1]
                destination = None


def diagnose(root, home):
    checks = []

    def add(status, name, detail):
        checks.append({"status": status, "name": name, "detail": detail})

    links = list(managed_links(root))
    if not links:
        add("FAIL", "managed links", "No supported link entries found in steps/*.yml")
    for destination, source in links:
        target = home / destination[2:]
        severity = "FAIL" if destination in CRITICAL else "WARN"
        try:
            if not source.exists():
                add(severity, destination, "Managed source is missing")
            elif not target.is_symlink():
                add(
                    severity,
                    destination,
                    "Missing managed symlink"
                    if not target.exists()
                    else "Exists but is not a managed symlink",
                )
            elif target.resolve(strict=True) != source.resolve(strict=True):
                add(
                    severity,
                    destination,
                    "Symlink points outside its configured source",
                )
            else:
                add("OK", destination, "Managed link resolves correctly")
        except (OSError, RuntimeError):
            add(severity, destination, "Broken or cyclic symlink")

    for command in (
        "zsh",
        "git",
        "python3",
        "tmux",
        "sesh",
        "nvim",
        "fzf",
        "dots",
        "gum",
        "zoxide",
        "eza",
        "bat",
        "rg",
        "fd",
        "yazi",
        "lazygit",
        "atuin",
    ):
        found = shutil.which(command)
        add(
            "OK"
            if found
            else ("FAIL" if command in ("zsh", "git", "python3", "tmux") else "WARN"),
            f"command:{command}",
            "Available" if found else "Not on PATH",
        )

    zshrc = root / "config/zsh/.zshrc"
    if zshrc.is_file():
        content = zshrc.read_text()
        if "alias deploy-dev=" in content:
            # Resolve the literal alias, rather than assuming the checkout is ~/dots.
            match = re.search(r"alias deploy-dev=['\"]([^'\"]+)['\"]", content)
            if match:
                script = match[1].replace("~/", str(home) + "/", 1)
                add(
                    "OK" if Path(script).is_file() else "WARN",
                    "alias:deploy-dev",
                    "Script exists"
                    if Path(script).is_file()
                    else "Alias references a missing script",
                )
        if "alias nvim-bench=" in content:
            add(
                "OK" if shutil.which("hyperfine") else "WARN",
                "alias:nvim-bench",
                "hyperfine available"
                if shutil.which("hyperfine")
                else "Requires missing hyperfine",
            )
        zsh = shutil.which("zsh")
        if zsh:
            try:
                result = subprocess.run(
                    [zsh, "-f", "-n", str(zshrc)],
                    capture_output=True,
                    check=False,
                    timeout=5,
                    env={**os.environ, "ZDOTDIR": "/dev/null"},
                )
                add(
                    "OK" if result.returncode == 0 else "FAIL",
                    "syntax:zshrc",
                    "zsh -f -n passed"
                    if result.returncode == 0
                    else "zsh syntax validation failed",
                )
            except (OSError, subprocess.TimeoutExpired):
                add(
                    "WARN",
                    "syntax:zshrc",
                    "Native syntax check unavailable or timed out",
                )

    # TOML parsers do not execute config and do not disclose values in diagnostics.
    config_files = [root / "config/sesh/sesh.toml", home / ".config/atuin/config.toml"]
    for file in config_files:
        name = f"syntax:{file.parent.name}/{file.name}"
        if not file.is_file():
            add("WARN", name, "Config missing")
        elif tomllib is None:
            add("WARN", name, "TOML validation requires Python 3.11+")
        else:
            try:
                parsed = tomllib.loads(file.read_text())
                add("OK", name, "TOML parsed")
                if file.parent.name == "atuin":
                    local = (
                        parsed.get("auto_sync") is False
                        and parsed.get("update_check") is False
                    )
                    add(
                        "OK" if local else "WARN",
                        "atuin:local-only",
                        "Sync and update checks disabled"
                        if local
                        else "Set auto_sync=false and update_check=false",
                    )
            except (OSError, ValueError):
                add("FAIL", name, "Cannot parse config")

    private = home / ".config/dots"
    if not private.is_dir():
        add("WARN", "private:dots", "Machine-local config directory missing")
    else:
        for filename in ("projects.json", "style", "ghostty-style.conf"):
            if not (private / filename).is_file():
                add("WARN", f"local:{filename}", "Machine-local setup not initialized")
        style = private / "style"
        if style.is_file():
            valid_style = style.read_text().strip() in (
                "glass",
                "focus",
                "presentation",
            )
            add(
                "OK" if valid_style else "FAIL",
                "style:mode",
                "Known style mode"
                if valid_style
                else "Expected glass, focus or presentation",
            )
        for path in [private, *sorted(private.iterdir())]:
            try:
                mode = stat.S_IMODE(path.stat().st_mode)
                safe = not mode & 0o077
                add(
                    "OK" if safe else "WARN",
                    f"private:{path.name}",
                    f"Mode {mode:03o}"
                    + ("" if safe else "; expected owner-only access"),
                )
                if path.suffix == ".json" and path.is_file():
                    try:
                        json.loads(path.read_text())
                        add("OK", f"syntax:{path.name}", "JSON parsed")
                    except (OSError, ValueError):
                        add("FAIL", f"syntax:{path.name}", "Cannot parse JSON")
            except OSError:
                add("WARN", f"private:{path.name}", "Cannot inspect metadata")

    # Inspect permissions only: never read key bytes, even to identify their format.
    ssh = home / ".ssh"
    if ssh.is_dir():
        for path in [ssh, *sorted(ssh.glob("id_*"))]:
            if path.suffix == ".pub":
                continue
            try:
                mode = stat.S_IMODE(path.stat().st_mode)
                add(
                    "WARN" if mode & 0o077 else "OK",
                    f"permissions:.ssh/{path.name}",
                    f"Mode {mode:03o}",
                )
            except OSError:
                add("WARN", f"permissions:.ssh/{path.name}", "Cannot inspect metadata")
    return checks


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--json", action="store_true", help="Print machine-readable checks and summary"
    )
    parser.add_argument(
        "--strict",
        action="store_true",
        help="Exit nonzero on warnings as well as failures",
    )
    args = parser.parse_args(argv)
    root = Path(os.environ.get("DOTS_ROOT", Path(__file__).resolve().parents[1]))
    try:
        checks = diagnose(root, Path.home())
    except OSError as error:
        checks = [
            {
                "status": "FAIL",
                "name": "doctor",
                "detail": f"Cannot inspect configuration ({type(error).__name__})",
            }
        ]
    summary = {
        status: sum(item["status"] == status for item in checks)
        for status in ("OK", "WARN", "FAIL")
    }
    if args.json:
        print(
            json.dumps(
                {"checks": checks, "summary": summary}, ensure_ascii=False, indent=2
            )
        )
    else:
        for item in checks:
            print(f"{item['status']:4} {item['name']}: {item['detail']}")
        print(
            "\n" + " / ".join(f"{status}: {count}" for status, count in summary.items())
        )
    return int(bool(summary["FAIL"] or (args.strict and summary["WARN"])))


if __name__ == "__main__":
    sys.exit(main())
