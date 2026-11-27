#!/bin/bash
set -euo pipefail
dir="$HOME/Library/LaunchAgents"
mkdir -p "$dir"
AGY_AGENTS="$dir" python3 - <<'PY'
import os, plistlib
from pathlib import Path
home = Path.home()
jobs = [
    {'Label': 'local.dots.antigravity-update',
     'ProgramArguments': ['/bin/bash', str(home / '.config/dots/agy-install-update.sh')],
     'StartInterval': 86400,
     'EnvironmentVariables': {'PATH': '/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin'}},
    # ssh -L to the agy proxy; exits 0 (no restart) unless ~/.config/agy/proxy.ssh and proxy.connect exist.
    {'Label': 'local.dots.agy-tunnel',
     'ProgramArguments': ['/usr/bin/python3', '-I', str(home / '.local/bin/agy-manager'), '--dots-tunnel'],
     'RunAtLoad': True,
     'WatchPaths': [str(home / '.config/agy' / n) for n in ('proxy.ssh', 'proxy.connect')],
     'KeepAlive': {'SuccessfulExit': False},
     'ThrottleInterval': 30},
]
for job in jobs:
    path = Path(os.environ['AGY_AGENTS']) / f"{job['Label']}.plist"
    if path.is_symlink():
        raise SystemExit('agy: refusing LaunchAgent symlink')
    if path.exists():
        with path.open('rb') as f:
            if plistlib.load(f) != job:
                raise SystemExit(f"agy: refusing to replace different LaunchAgent {path.name}")
    else:
        with path.open('xb') as f:
            plistlib.dump(job, f)
PY
for label in local.dots.antigravity-update local.dots.agy-tunnel; do
  if ! launchctl print "gui/$(id -u)/$label" >/dev/null 2>&1; then
    launchctl bootstrap "gui/$(id -u)" "$dir/$label.plist"
  fi
done
