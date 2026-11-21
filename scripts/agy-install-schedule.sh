#!/bin/bash
set -euo pipefail
label=local.dots.antigravity-update
agent="$HOME/Library/LaunchAgents/$label.plist"
mkdir -p "${agent%/*}"
AGY_AGENT="$agent" python3 - <<'PY'
import os, plistlib
from pathlib import Path
path = Path(os.environ['AGY_AGENT'])
job = {'Label': 'local.dots.antigravity-update',
       'ProgramArguments': ['/bin/bash', str(Path.home() / '.config/dots/agy-install-update.sh')],
       'StartInterval': 86400,
       'EnvironmentVariables': {'PATH': '/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin'}}
if path.is_symlink():
    raise SystemExit('agy: refusing LaunchAgent symlink')
if path.exists():
    with path.open('rb') as f:
        if plistlib.load(f) != job:
            raise SystemExit('agy: refusing to replace different LaunchAgent')
else:
    with path.open('xb') as f:
        plistlib.dump(job, f)
PY
if ! launchctl print "gui/$(id -u)/$label" >/dev/null 2>&1; then
  launchctl bootstrap "gui/$(id -u)" "$agent"
fi
