import json
import os
from pathlib import Path
import socket
import shutil
import subprocess
import tempfile
import time
import unittest
from unittest import mock
import uuid

import browser

HERE = Path(__file__).resolve().parent


class BrowserTests(unittest.TestCase):
    def test_start_opens_local_viewer_after_success(self):
        config = {'proxy_url': 'http://private-user:private-password@192.0.2.1:8080', 'browser_port': 5917}
        with tempfile.TemporaryDirectory() as temp, mock.patch.object(browser, 'start') as start, \
                mock.patch.object(browser.subprocess, 'run') as run:
            run.side_effect = lambda *args, **kwargs: start.assert_called_once()
            browser.main(config, HERE, Path(temp), ['start'])
            run.assert_called_once_with(['/usr/bin/open', '-b', 'com.apple.ScreenSharing', 'vnc://127.0.0.1:5917'],
                                        stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                        check=True, timeout=10, close_fds=True)
            self.assertNotIn('private-', str(run.call_args))

    def test_failed_start_never_opens_viewer(self):
        with tempfile.TemporaryDirectory() as temp, \
                mock.patch.object(browser, 'start', side_effect=RuntimeError('guard failed')), \
                mock.patch.object(browser.subprocess, 'run') as run:
            with self.assertRaisesRegex(RuntimeError, 'guard failed'):
                browser.main({'proxy_url': 'http://192.0.2.1:8080'}, HERE, Path(temp), ['start'])
            run.assert_not_called()

    def test_viewer_failure_keeps_browser_and_reports_local_fallback(self):
        for error in (OSError('unavailable'), subprocess.CalledProcessError(1, ['open']),
                      subprocess.TimeoutExpired(['open'], 10)):
            with self.subTest(error=type(error).__name__), tempfile.TemporaryDirectory() as temp, \
                    mock.patch.object(browser, 'start'), mock.patch.object(browser, 'cleanup') as cleanup, \
                    mock.patch.object(browser.subprocess, 'run', side_effect=error):
                with self.assertRaisesRegex(RuntimeError, 'Browser is running.*vnc://127.0.0.1:5909'):
                    browser.main({'proxy_url': 'http://192.0.2.1:8080'}, HERE, Path(temp), ['start'])
                cleanup.assert_not_called()

    def test_reused_browser_opens_viewer_without_restarting(self):
        config = {'proxy_url': 'http://192.0.2.1:8080'}
        guard = {'Id': 'guard-id', 'HostConfig': {'PortBindings': {'5900/tcp': [{'HostPort': '5909'}]}}}
        existing = {'State': {'Running': True}, 'Config': {'User': '1000:1000'},
                    'HostConfig': {'NetworkMode': 'container:guard-id', 'ReadonlyRootfs': True,
                                   'CapDrop': ['ALL'], 'Memory': 1024 * 1024 * 1024, 'NanoCpus': 2000000000}}
        with tempfile.TemporaryDirectory() as temp, mock.patch.object(browser, 'owned', return_value=existing), \
                mock.patch.object(browser, 'guard_ready', return_value=guard), \
                mock.patch.object(browser, 'cleanup') as cleanup, mock.patch.object(browser, 'docker') as docker, \
                mock.patch.object(browser.subprocess, 'run') as run:
            (Path(temp) / 'browser.json').write_text(json.dumps({
                'proxy': browser.hashlib.sha256(config['proxy_url'].encode()).hexdigest(),
                'port': 5909, 'automatic_auth': 1}))
            browser.main(config, HERE, Path(temp), ['start'])
            run.assert_called_once()
            cleanup.assert_not_called()
            docker.assert_not_called()
            run.reset_mock()
            with self.assertRaisesRegex(RuntimeError, 'configuration changed'):
                browser.main(dict(config, browser_port=5917), HERE, Path(temp), ['start'])
            run.assert_not_called()

    def test_firefox_auth_scope(self):
        node = shutil.which('node')
        if not node:
            self.skipTest('requires installed Node.js')
        subprocess.run([node, str(HERE / 'test_firefox_auth.js'), str(HERE / 'firefox.cfg')], check=True, timeout=15)

    def test_proxy_credentials_use_stdin_only(self):
        config = {'proxy_url': 'http://canary%40user:p%22%5C%3A%40@192.0.2.1:8080'}
        for ready in (True, False):
            with self.subTest(ready=ready), tempfile.TemporaryDirectory() as temp, \
                    mock.patch.object(browser, 'owned', side_effect=lambda kind, name: None if kind == 'container' else {'owned': True}), \
                    mock.patch.object(browser, 'start_guard'), mock.patch.object(browser, 'cleanup') as cleanup, \
                    mock.patch.object(browser.time, 'sleep'), mock.patch.object(browser, 'docker') as docker, \
                    mock.patch.object(browser.subprocess, 'Popen') as popen:
                popen.return_value.poll.return_value = None
                def result(arguments, **kwargs):
                    if arguments[:2] == ['exec', browser.BROWSER] and not ready:
                        raise RuntimeError('not ready')
                    return ''
                docker.side_effect = result
                if ready:
                    browser.start(config, HERE, Path(temp))
                    self.assertEqual(json.loads((Path(temp) / 'browser.json').read_text())['automatic_auth'], 1)
                else:
                    with self.assertRaisesRegex(RuntimeError, 'readiness timed out'):
                        browser.start(config, HERE, Path(temp))
                    self.assertFalse((Path(temp) / 'browser.json').exists())
                    self.assertEqual(cleanup.call_count, 2)
                payload = popen.return_value.stdin.write.call_args.args[0].decode().splitlines()
                self.assertEqual(len(payload), 2)
                self.assertEqual(json.loads(payload[1]), {'username': 'canary@user', 'password': 'p"\\:@'})
                for call in docker.call_args_list:
                    self.assertNotIn('canary', str(call))
                self.assertNotIn('canary', str(popen.call_args))
                popen.return_value.stdin.close.assert_called_once()
                created = next(call.args[0] for call in docker.call_args_list if call.args[0][0] == 'create')
                self.assertIn('none', created[created.index('--log-driver') + 1:])
                self.assertTrue(any('dst=/usr/lib/firefox/mozilla.cfg,readonly' in value for value in created))

    @unittest.skipUnless(os.environ.get('CLAUDE_GUARD_DOCKER_TESTS') == '1', 'requires opted-in Docker tests')
    def test_profile_volume_keeps_browser_ownership(self):
        volume = 'claude-profile-test-' + uuid.uuid4().hex[:12]
        browser.docker(['volume', 'create', volume])
        try:
            common = ['run', '--rm', '--pull', 'never', '--network', 'none', '--read-only',
                      '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges',
                      '--mount', 'type=volume,src={},dst=/profile,volume-nocopy'.format(volume), '--entrypoint', '/bin/sh']
            browser.docker(common + ['--cap-add', 'CHOWN', 'moby/buildkit:buildx-stable-1', '-c', 'chown 1000:1000 /profile'])
            result = browser.docker(common + ['--user', '1000:1000', 'moby/buildkit:buildx-stable-1', '-c',
                                             'mkdir /profile/firefox && stat -c "%u:%g" /profile'])
            self.assertEqual(result, '1000:1000')
            self.assertEqual(Path(browser.__file__).read_text().count('src={},dst=/profile,volume-nocopy'), 2)
        finally:
            browser.docker(['volume', 'rm', volume])

    def test_missing_volume_is_not_an_inspection_failure(self):
        with mock.patch.object(browser.subprocess, 'run') as run:
            run.return_value.returncode = 1
            run.return_value.stdout = '[]'
            run.return_value.stderr = 'Error response from daemon: get dots-claude-browser-profile: no such volume'
            self.assertIsNone(browser.owned('volume', browser.VOLUME))
            run.return_value.stderr = 'Error response from daemon: permission denied'
            with self.assertRaises(RuntimeError):
                browser.owned('volume', browser.VOLUME)

    def test_docker_is_pinned_to_local_socket(self):
        with mock.patch.object(browser.subprocess, 'run') as run:
            run.return_value.returncode = 0
            run.return_value.stdout = '[]'
            browser.docker(['container', 'ls'])
            command = run.call_args.args[0]
            self.assertEqual(command[:3], ['/usr/local/bin/docker', '--host',
                                          'unix://' + str(Path.home() / '.docker/run/docker.sock')])
        self.assertIn('subprocess.Popen(DOCKER +', Path(browser.__file__).read_text())

    def test_proxy_and_resource_validation(self):
        valid = {"proxy_url": "http://192.0.2.1:8080"}
        self.assertEqual(browser.options(valid)[2:5], (1024, 2, 5909))
        for changed in ({"proxy_url": ""}, {"proxy_url": "http://localhost:8080"},
                        {"proxy_url": "http://127.0.0.1:8080"}, {"browser_memory_mb": 0},
                        {"browser_cpus": float("nan")}, {"browser_cpus": True}, {"browser_port": 80}):
            with self.subTest(changed=changed), self.assertRaises(ValueError):
                browser.options(dict(valid, **changed))

    def test_absent_proxy_never_calls_docker(self):
        with mock.patch.object(browser, "docker") as docker:
            for command in ("start", "prepare"):
                with self.assertRaises(ValueError):
                    browser.main({"proxy_url": ""}, HERE, HERE, [command])
            docker.assert_not_called()

    def test_foreign_resources_are_not_adopted_or_removed(self):
        with mock.patch.object(browser, "docker", return_value=json.dumps([{"Config": {"Labels": {}}}])) as docker:
            with self.assertRaises(RuntimeError):
                browser.cleanup()
            self.assertEqual(len(docker.call_args_list), 1)
            self.assertEqual(docker.call_args.args[0][:2], ["container", "inspect"])

    def test_no_implicit_image_pull_or_credential_env(self):
        source = Path(browser.__file__).read_text()
        self.assertNotIn('"pull", IMAGE', source)
        self.assertIn('"--pull", "never"', source)
        self.assertNotIn('"--env", "HTTP_PROXY="', source)
        self.assertNotIn("--no-sandbox", source)

    @unittest.skipUnless(os.environ.get("CLAUDE_GUARD_DOCKER_TESTS") == "1", "requires opted-in Docker tests")
    def test_real_manager_guard_configuration(self):
        with socket.socket() as reserve:
            reserve.bind(("127.0.0.1", 0))
            port = reserve.getsockname()[1]
        name = "claude-manager-test-" + uuid.uuid4().hex[:12]
        workload = name + "-workload"
        created_workload = False
        with mock.patch.object(browser, "GUARD", name), mock.patch.object(browser, "NETWORK", name + "-net"):
            try:
                proxy = browser.validate_proxy("http://192.0.2.1:8080")
                browser.start_guard(HERE, proxy, port, "moby/buildkit:buildx-stable-1")
                info = browser.guard_ready()
                self.assertTrue(info["State"]["Running"])
                browser.docker(["run", "-d", "--pull", "never", "--name", workload,
                                "--network", "container:" + name, "--cap-drop", "ALL", "--user", "1000:1000",
                                "--read-only", "--security-opt", "no-new-privileges", "--entrypoint", "/bin/sh",
                                "moby/buildkit:buildx-stable-1", "-c", "while true; do nc -l -p 5900 -e /bin/cat; done"])
                created_workload = True
                for attempt in range(30):
                    try:
                        with socket.create_connection(("127.0.0.1", port), timeout=2) as client:
                            client.sendall(b"local-vnc")
                            self.assertEqual(client.recv(9), b"local-vnc")
                        break
                    except (OSError, AssertionError):
                        if attempt == 29:
                            raise
                        time.sleep(0.1)
                # A default DROP policy alone is not a sufficient readiness check.
                browser.docker(["exec", name, "iptables", "-A", "OUTPUT", "-j", "ACCEPT"])
                with self.assertRaises(RuntimeError):
                    browser.guard_ready()
            finally:
                if created_workload:
                    browser.docker(["rm", "-f", "-v", workload])
                if browser.owned("container", name):
                    browser.docker(["rm", "-f", "-v", name])
                if browser.owned("network", browser.NETWORK):
                    browser.docker(["network", "rm", browser.NETWORK])


if __name__ == "__main__":
    unittest.main(verbosity=2)
