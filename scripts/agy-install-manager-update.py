#!/usr/bin/python3
import ctypes
import os
from pathlib import Path
import plistlib
import re
import signal
import subprocess
import sys

LABEL = 'local.dots.antigravity-manager'
CASK = 'lbjlaq/antigravity-manager/antigravity-tools'
SERVER = '/Applications/Antigravity Tools.app/Contents/MacOS/antigravity-server'
GUI = '/Applications/Antigravity Tools.app/Contents/MacOS/antigravity-tools'
EXPECTED = {'Label': LABEL, 'ProgramArguments': ['/bin/sh', '-c', 'exec "$HOME/.local/bin/agy-manager" --dots-service'], 'RunAtLoad': True}


def run(*args):
    return subprocess.run(args, text=True, capture_output=True, stdin=subprocess.DEVNULL, timeout=120)


def process_path(pid):
    buf = ctypes.create_string_buffer(4096)
    lib = ctypes.CDLL('/usr/lib/libproc.dylib')
    if lib.proc_pidpath(pid, buf, len(buf)) <= 0:
        raise ValueError('cannot verify service executable')
    return os.fsdecode(buf.value)


def manager_update():
    if run('brew', 'list', '--cask', CASK).returncode:
        return
    if run('brew', 'update', '--quiet').returncode:
        raise ValueError('cannot refresh Homebrew metadata')
    outdated = run('brew', 'outdated', '--cask', '--quiet', CASK)
    if outdated.returncode:
        raise ValueError('cannot check Manager updates')
    if not outdated.stdout.strip():
        return
    domain = f'gui/{os.getuid()}'
    target = f'{domain}/{LABEL}'
    path = Path.home() / 'Library/LaunchAgents' / (LABEL + '.plist')
    result = run('launchctl', 'print', target)
    gui = run('pgrep', '-f', '[A]ntigravity Tools.app/Contents/MacOS/antigravity-tools')
    if gui.returncode not in (0, 1):
        raise ValueError('cannot inspect GUI')
    if gui.returncode == 0:
        print('Manager update deferred: GUI running')
        return
    stopped = False
    try:
        if result.returncode == 0:
            with path.open('rb') as f:
                if plistlib.load(f) != EXPECTED:
                    raise ValueError('foreign Manager plist')
            if f'path = {path}' not in result.stdout or 'program = /bin/sh' not in result.stdout or EXPECTED['ProgramArguments'][2] not in result.stdout:
                raise ValueError('foreign loaded Manager job')
            match = re.search(r'^\s*pid = (\d+)\s*$', result.stdout, re.MULTILINE)
            if not match:
                raise ValueError('Manager job has no verifiable running process')
            pid = int(match.group(1))
            uid = run('ps', '-p', str(pid), '-o', 'uid=')
            if uid.returncode or uid.stdout.strip() != str(os.getuid()) or process_path(pid) != SERVER:
                raise ValueError('foreign Manager process')
            connections = run('lsof', '-nP', '-a', '-p', str(pid), '-iTCP', '-sTCP:ESTABLISHED')
            if connections.returncode not in (0, 1) or connections.stderr:
                raise ValueError('cannot inspect Manager connections')
            if connections.returncode == 0:
                print('Manager update deferred: active connections')
                return
            stopped = True
            if run('launchctl', 'bootout', target).returncode:
                raise ValueError('cannot stop owned Manager')
        else:
            if run('launchctl', 'print', domain).returncode:
                raise ValueError('cannot inspect launchd domain')
            server = run('pgrep', '-f', '[A]ntigravity Tools.app/Contents/MacOS/antigravity-server')
            if server.returncode != 1:
                raise ValueError('unowned Manager process or unable to inspect')
        result = subprocess.run(['brew', 'upgrade', '--cask', CASK], stdin=subprocess.DEVNULL, timeout=1800)
        if result.returncode:
            raise ValueError('Manager upgrade failed')
    finally:
        if stopped and run('launchctl', 'print', target).returncode and run('launchctl', 'bootstrap', domain, str(path)).returncode:
            raise ValueError('cannot restore Manager job')


def interrupted(signum, frame):
    raise KeyboardInterrupt


if __name__ == '__main__':
    signal.signal(signal.SIGTERM, interrupted)
    try:
        manager_update()
    except (OSError, ValueError, subprocess.SubprocessError, KeyboardInterrupt) as error:
        print(f'Manager update: {error}', file=sys.stderr)
        sys.exit(1)
