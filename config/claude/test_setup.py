import concurrent.futures
import contextlib
import json
import os
from pathlib import Path
import socket
import socketserver
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from unittest import mock

import guard
import install_local
import launcher
import relay

HERE = Path(__file__).resolve().parent


class Echo(socketserver.BaseRequestHandler):
    def handle(self):
        while True:
            data = self.request.recv(65536)
            if not data:
                return
            self.request.sendall(data)


@contextlib.contextmanager
def running_relay(url, directory):
    code = "import runpy,sys;sys.path.insert(0,sys.argv.pop(1));runpy.run_module('relay',run_name='__main__')"
    process = subprocess.Popen([sys.executable, "-I", "-c", code, str(HERE), str(directory)],
                               stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
    process.stdin.write(json.dumps({"proxy_url": url}).encode() + b"\n")
    process.stdin.close()
    status = None
    try:
        for _ in range(100):
            try:
                with socket.socket(socket.AF_UNIX) as control:
                    control.settimeout(1)
                    control.connect(str(directory / "control.sock"))
                    control.sendall(b"status\n")
                    status = json.loads(control.recv(4096))
                break
            except (OSError, ValueError):
                if process.poll() is not None:
                    raise AssertionError(process.stderr.read().decode())
                time.sleep(0.03)
        if status is None:
            raise AssertionError("relay readiness timeout")
        yield status
    finally:
        if process.poll() is None:
            process.terminate()
        process.wait(timeout=5)
        process.stderr.close()


class SetupTests(unittest.TestCase):
    def test_missing_proxy_never_starts_service_or_install(self):
        with mock.patch.object(launcher, "relay") as start, mock.patch.object(subprocess, "run") as run:
            with self.assertRaises(ValueError):
                launcher.install_claude({"proxy_url": ""})
            with self.assertRaises(ValueError):
                launcher.run_claude([], {"proxy_url": ""})
            start.assert_not_called()
            run.assert_not_called()

    def test_proxy_errors_do_not_echo_credentials(self):
        with self.assertRaises(ValueError) as caught:
            guard.validate_proxy("http://user:super-secret")
        self.assertNotIn("super-secret", str(caught.exception))

    def test_local_install_preserves_foreign_setup_and_is_idempotent(self):
        with tempfile.TemporaryDirectory(dir=HERE) as temporary:
            home = Path(temporary)
            skill = home / ".agents/skills/ponytail"
            skill.mkdir(parents=True)
            (skill / "SKILL.md").write_text("test skill")
            forbidden = home / ".agents/skills/chief-of-staff"
            forbidden.mkdir()
            (forbidden / "SKILL.md").write_text("not transferred")
            old = home / ".claude/settings.json"
            old.parent.mkdir()
            old.write_text('{"existing": true}')
            with contextlib.redirect_stdout(__import__("io").StringIO()):
                install_local.install(home)
                install_local.install(home)
            self.assertEqual(json.loads(old.read_text()), {"existing": True})
            base = home / ".local/share/claude-guard"
            self.assertTrue((base / "home/.claude/skills/ponytail").is_symlink())
            self.assertFalse((base / "home/.claude/skills/chief-of-staff").exists())
            self.assertFalse((base / "home/.claude/agents").exists())
            self.assertEqual((base / "runtime/browser-start.sh").stat().st_mode & 0o777, 0o644)
            self.assertEqual((base / "runtime/firewall.sh").stat().st_mode & 0o777, 0o644)
            settings = home / ".config/claude-guard/settings.json"
            self.assertEqual(settings.stat().st_mode & 0o777, 0o600)
            self.assertEqual(json.loads(settings.read_text())["proxy_url"], "")
            result = subprocess.run([str(home / ".local/bin/claude-setup"), "check"],
                                    env={"HOME": str(home), "PATH": "/usr/bin:/bin"}, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)

    def test_relay_both_families_concurrency_and_upstream_failure(self):
        with tempfile.TemporaryDirectory(dir=HERE) as temporary:
            directory = Path(temporary)
            server = socketserver.ThreadingTCPServer(("127.0.0.1", 0), Echo)
            server.daemon_threads = True
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            url = "http://127.0.0.1:{}".format(server.server_address[1])
            try:
                with running_relay(url, directory) as status:
                    self.assertEqual(status["fingerprint"], relay.fingerprint(url))
                    def exchange(index):
                        family = socket.AF_INET6 if index % 2 else socket.AF_INET
                        address = "::1" if index % 2 else "127.0.0.1"
                        with socket.socket(family) as connection:
                            connection.settimeout(3)
                            connection.connect((address, status["port"]))
                            payload = ("channel-{}".format(index)).encode()
                            connection.sendall(payload)
                            self.assertEqual(connection.recv(100), payload)
                    with concurrent.futures.ThreadPoolExecutor(max_workers=12) as pool:
                        list(pool.map(exchange, range(24)))
                    server.shutdown()
                    server.server_close()
                    with socket.create_connection(("127.0.0.1", status["port"]), timeout=3) as connection:
                        connection.sendall(b"must not fall back")
                        try:
                            self.assertEqual(connection.recv(100), b"")
                        except ConnectionResetError:
                            pass
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=3)

    @unittest.skipUnless(sys.platform == "darwin", "requires macOS")
    def test_full_guard_and_relay_path(self):
        with tempfile.TemporaryDirectory(dir=HERE) as temporary:
            directory = Path(temporary)
            state, work = directory / "state", directory / "work"
            state.mkdir()
            work.mkdir()
            with socketserver.ThreadingTCPServer(("127.0.0.1", 0), Echo) as upstream:
                upstream.daemon_threads = True
                threading.Thread(target=upstream.serve_forever, daemon=True).start()
                try:
                    url = "http://127.0.0.1:{}".format(upstream.server_address[1])
                    with running_relay(url, state) as status:
                        profile = guard.make_profile(status["port"], [work], [state, HERE])
                        code = "import socket,sys; s=socket.create_connection(('127.0.0.1',int(sys.argv[1])));s.sendall(b'guarded');assert s.recv(7)==b'guarded'"
                        result = subprocess.run(guard.sandbox_argv(profile, [sys.executable, "-c", code, str(status["port"])]),
                                                env=guard.proxy_env(launcher.local_proxy(url, status["port"]), work, work),
                                                capture_output=True, text=True, timeout=10)
                        self.assertEqual(result.returncode, 0, result.stderr)
                        denied = subprocess.run(guard.sandbox_argv(profile, [sys.executable, "-c", code, str(upstream.server_address[1])]),
                                                env={}, capture_output=True, text=True, timeout=10)
                        self.assertNotEqual(denied.returncode, 0)
                    dead = subprocess.run(guard.sandbox_argv(profile, [sys.executable, "-c", code, str(status["port"])]),
                                          env={}, capture_output=True, text=True, timeout=10)
                    self.assertNotEqual(dead.returncode, 0, "stopped relay must fail closed")
                finally:
                    upstream.shutdown()


if __name__ == "__main__":
    unittest.main(verbosity=2)
