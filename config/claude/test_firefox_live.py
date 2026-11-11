import base64
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import unittest
import uuid

HERE = Path(__file__).resolve().parent
DOCKER = ['/usr/local/bin/docker', '--host', f'unix://{Path.home()}/.docker/run/docker.sock']
IMAGE = 'dots-claude-browser:local'


def canary(mode):
    os.umask(0o077)
    user, password = 'test!$&\'"\\user', 'p@ss !:$&\'"\\word'
    expected = 'Basic ' + base64.b64encode((user + ':' + password).encode()).decode()
    os.makedirs('/run/guard', mode=0o700)
    with open('/run/guard/proxy-auth.json', 'w') as f:
        json.dump({'username': user, 'password': password}, f, ensure_ascii=True)
    sock = socket.socket()
    sock.bind(('127.0.0.1', 0))
    sock.listen(20)
    sock.settimeout(.2)
    os.environ.update(HOME=tempfile.mkdtemp(prefix='firefox-home-'), GUARD_PROXY_HOST='127.0.0.1',
                      GUARD_PROXY_PORT=str(sock.getsockname()[1]), GUARD_PROXY_TLS='0', MOZ_HEADLESS='1')
    requests, origin_requests = [], []
    stop = threading.Event()
    origin = socket.socket()
    origin.bind(('127.0.0.1', 0))
    origin.listen(5)
    origin.settimeout(.2)

    def serve_origin():
        while not stop.is_set():
            try:
                conn, _ = origin.accept()
            except socket.timeout:
                continue
            with conn:
                data = b''
                while b'\r\n\r\n' not in data:
                    part = conn.recv(4096)
                    if not part:
                        break
                    data += part
                origin_requests.append(data.decode('iso-8859-1'))
                conn.sendall(b'HTTP/1.1 401 Unauthorized\r\nWWW-Authenticate: Basic realm="canary-site"\r\nContent-Length: 0\r\nConnection: close\r\n\r\n')

    def serve_proxy():
        while not stop.is_set():
            try:
                conn, _ = sock.accept()
            except socket.timeout:
                continue
            with conn:
                conn.settimeout(1)
                data = b''
                try:
                    while b'\r\n\r\n' not in data and len(data) < 65536:
                        part = conn.recv(4096)
                        if not part:
                            break
                        data += part
                except OSError:
                    continue
                lines = data.decode('iso-8859-1').split('\r\n')
                headers = {}
                for line in lines[1:]:
                    if ':' in line:
                        k, v = line.split(':', 1)
                        headers[k.lower()] = v.strip()
                requests.append((lines[0], headers))
                if headers.get('proxy-authorization') != expected or mode == 'bad':
                    response = b'HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic realm="canary-proxy"\r\nContent-Length: 0\r\nConnection: close\r\n\r\n'
                elif mode == 'site401':
                    with socket.create_connection(origin.getsockname()) as upstream:
                        forwarded = [line for line in lines if not line.lower().startswith('proxy-authorization:')]
                        upstream.sendall('\r\n'.join(forwarded).encode('iso-8859-1'))
                        response = upstream.recv(4096)
                else:
                    body = b'<html><body>proxy canary success</body></html>'
                    response = b'HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: ' + str(len(body)).encode() + b'\r\nConnection: close\r\n\r\n' + body
                try:
                    conn.sendall(response)
                except OSError:
                    pass

    threading.Thread(target=serve_origin, daemon=True).start()
    threading.Thread(target=serve_proxy, daemon=True).start()
    profile = Path(os.environ['HOME']) / 'profile'
    profile.mkdir()
    screenshot = Path(os.environ['HOME']) / 'shot.png'
    cmd = ['firefox', '--headless', '--no-remote', '--profile', str(profile), '--screenshot', str(screenshot), 'http://example.invalid/canary']
    process = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, start_new_session=True)
    # Bound the observation window. Forced cleanup does not prove clean Firefox shutdown.
    try:
        output = process.communicate(timeout=15)[0]
        timed_out = False
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        output = process.communicate()[0]
        timed_out = True
    stop.set()
    relevant = [(line, h) for line, h in requests if 'example.invalid/canary' in line]
    print('mode=', mode, 'timeout=', timed_out, 'exit=', process.returncode,
          'ready=', Path('/run/guard/auth-ready').exists(),
          'credential_removed=', not Path('/run/guard/proxy-auth.json').exists(),
          'screenshot=', screenshot.exists(), flush=True)
    for line, h in relevant:
        print('request', line, 'proxy_auth=', 'expected' if h.get('proxy-authorization') == expected else
              ('other' if 'proxy-authorization' in h else 'absent'),
              'site_auth=', 'present' if 'authorization' in h else 'absent', flush=True)
    assert Path('/run/guard/auth-ready').exists() and not Path('/run/guard/proxy-auth.json').exists(), 'autoconfig failed'
    assert relevant and 'proxy-authorization' not in relevant[0][1]
    assert any(h.get('proxy-authorization') == expected for _, h in relevant), 'proxy credential never sent'
    assert all('authorization' not in h for _, h in relevant), 'website authorization unexpectedly sent'
    if mode == 'ok':
        assert screenshot.exists(), 'no screenshot'
    if mode == 'site401':
        assert origin_requests and all('authorization:' not in request.lower() for request in origin_requests), 'origin received credentials or no request'
    if mode == 'bad':
        assert len(relevant) <= 3, 'unbounded retry'
    needles = [user.encode(), password.encode(), expected.encode(), expected[6:].encode()]
    leaks = []
    for root, _, files in os.walk(os.environ['HOME']):
        for name in files:
            path = Path(root) / name
            if not path.is_symlink() and any(needle in path.read_bytes() for needle in needles):
                leaks.append(str(path))
    if any(needle in output for needle in needles):
        leaks.append('captured-firefox-output')
    print('profile and captured Firefox output matches=', len(leaks), flush=True)
    assert not leaks, f'canary credential persisted or logged: {leaks}'
    print('PASS', mode, flush=True)


@unittest.skipUnless(os.environ.get('CLAUDE_GUARD_DOCKER_TESTS') == '1', 'set CLAUDE_GUARD_DOCKER_TESTS=1 to run')
class FirefoxLiveTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        try:
            result = subprocess.run(DOCKER + ['image', 'inspect', IMAGE], stdout=subprocess.DEVNULL,
                                    stderr=subprocess.DEVNULL, timeout=15)
        except (OSError, subprocess.TimeoutExpired) as exc:
            raise unittest.SkipTest(f'local Docker unavailable: {exc}')
        if result.returncode:
            raise unittest.SkipTest('cached browser image unavailable')

    def test_canaries(self):
        for mode in ('ok', 'site401', 'bad'):
            with self.subTest(mode=mode):
                name = 'claude-auth-canary-' + uuid.uuid4().hex
                cmd = DOCKER + ['run', '--name', name, '--pull', 'never', '--network', 'none',
                    '--user', '1000:1000', '--cap-drop', 'ALL', '--read-only',
                    '--security-opt', 'no-new-privileges', '--log-driver', 'none',
                    '--tmpfs', '/tmp:rw,nosuid,nodev,size=256m', '--tmpfs', '/run:rw,nosuid,nodev,size=16m,mode=1777',
                    '--shm-size', '256m', '--memory', '1g', '--memory-swap', '1g', '--cpus', '2', '--pids-limit', '256',
                    '--entrypoint', '/usr/bin/python3',
                    '-v', f'{HERE / "firefox-prefs.js"}:/usr/lib/firefox/defaults/pref/guard.js:ro',
                    '-v', f'{HERE / "firefox.cfg"}:/usr/lib/firefox/mozilla.cfg:ro',
                    '-v', f'{Path(__file__).resolve()}:/canary.py:ro', IMAGE, '/canary.py', '--canary', mode]
                try:
                    result = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, timeout=35)
                    print(result.stdout, flush=True)
                    self.assertEqual(result.returncode, 0, result.stdout)
                finally:
                    subprocess.run(DOCKER + ['rm', '-f', '-v', name], stdout=subprocess.DEVNULL,
                                   stderr=subprocess.DEVNULL, timeout=15)


if __name__ == '__main__':
    if len(sys.argv) == 3 and sys.argv[1] == '--canary':
        canary(sys.argv[2])
    else:
        unittest.main()
