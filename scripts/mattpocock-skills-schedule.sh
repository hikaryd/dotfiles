#!/bin/bash
set -euo pipefail

label=local.dots.mattpocock-skills-update
agent="$HOME/Library/LaunchAgents/$label.plist"
mkdir -p "${agent%/*}"
mkdir -p -m 700 "$HOME/.local/state/dots"

MP_AGENT="$agent" python3 -I - <<'PY'
import os
import plistlib
import stat
from pathlib import Path

def check_dir(directory, private=False):
    info = directory.lstat()
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o022:
        raise SystemExit(f"mattpocock-skills: unsafe directory: {directory}")
    if private and (info.st_mode & 0o077):
        os.chmod(directory, 0o700)

path = Path(os.environ["MP_AGENT"])
home = Path.home()
check_dir(path.parent, private=False)
state_dir = home / ".local/state/dots"
check_dir(state_dir, private=True)

log_path = state_dir / "mattpocock-skills-update.log"
if log_path.is_symlink():
    raise SystemExit("mattpocock-skills: refusing log symlink")

job = {
    "Label": "local.dots.mattpocock-skills-update",
    "ProgramArguments": ["/bin/bash", str(home / ".config/dots/mattpocock-skills-update.sh")],
    "StartInterval": 86400,
    "StandardOutPath": str(log_path),
    "StandardErrorPath": str(log_path),
    "EnvironmentVariables": {
        "PATH": "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
    },
}

try:
    info = path.lstat()
except FileNotFoundError:
    info = None

if info is not None:
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o022:
        raise SystemExit("mattpocock-skills: unsafe LaunchAgent file")
    with path.open("rb") as f:
        if plistlib.load(f) != job:
            raise SystemExit("mattpocock-skills: refusing to replace different LaunchAgent")
    if stat.S_IMODE(info.st_mode) != 0o644:
        os.chmod(path, 0o644)
else:
    with path.open("xb") as f:
        plistlib.dump(job, f)
    os.chmod(path, 0o644)
PY

if ! launchctl print "gui/$(id -u)/$label" >/dev/null 2>&1; then
  launchctl bootstrap "gui/$(id -u)" "$agent"
fi
