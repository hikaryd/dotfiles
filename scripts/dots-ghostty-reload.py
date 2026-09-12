#!/usr/bin/env python3
"""Мягкий reload уже работающего Ghostty; без activate/keys/new window."""

import subprocess
import sys


def main():
    if sys.platform != "darwin":
        return 0
    if subprocess.run(
        ["pgrep", "-x", "ghostty"], capture_output=True, check=False
    ).returncode:
        return 0
    # Команды и object model из установленного Ghostty.sdef (Ghostty 1.3.1).
    script = """
tell application "Ghostty"
  if (count of windows) is 0 then return false
  set targetTerminal to focused terminal of selected tab of front window
  if not (perform action "reload_config" on targetTerminal) then return false
  repeat with w in windows
    repeat with t in terminals of w
      perform action "reset_font_size" on t
    end repeat
  end repeat
  return true
end tell
"""
    try:
        result = subprocess.run(
            ["/usr/bin/osascript", "-e", script],
            capture_output=True,
            text=True,
            timeout=5,
            check=False,
        )
        if result.returncode == 0 and result.stdout.strip() == "true":
            print(
                "Ghostty: configuration reloaded, font size reset to profile default."
            )
            return 0
    except (OSError, subprocess.TimeoutExpired):
        pass
    print(
        "Ghostty: profile saved; live reload unavailable (Cmd+Shift+, then Cmd+0).",
        file=sys.stderr,
    )
    return 1


if __name__ == "__main__":
    sys.exit(main())
