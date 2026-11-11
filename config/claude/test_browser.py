import json
import os
from pathlib import Path
import socket
import time
import unittest
from unittest import mock
import uuid

import browser

HERE = Path(__file__).resolve().parent


class BrowserTests(unittest.TestCase):
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
