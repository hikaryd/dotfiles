"""No phone commands or server listener are started by these regression tests."""

import concurrent.futures
import email.message
import json
import pathlib
import shutil
import subprocess
import tempfile
import threading
import types
import unittest
from unittest import mock

import owner_control as control


class PrivateConfigTests(unittest.TestCase):
    def test_exact_origin_and_required_private_identity(self):
        valid = {
            "token": "a" * 48,
            "origin": "https://192.0.2.116:8443",
            "lan_ip": "192.0.2.116",
            "serial": "EXAMPLE_SERIAL",
        }
        with (
            tempfile.TemporaryDirectory() as tmp,
            mock.patch.object(control, "HOME", pathlib.Path(tmp)),
        ):
            target = pathlib.Path(tmp) / "pixel-admin/control.json"
            target.parent.mkdir()
            target.write_text(json.dumps(valid))
            self.assertEqual(control.read_config(), valid)
            for key, value in [
                ("origin", "https://192.0.2.117:8443"),
                ("origin", valid["origin"] + "/"),
                ("serial", ""),
                ("token", "short"),
                ("lan_ip", "0.0.0.0"),
                ("lan_ip", "127.0.0.1"),
                ("lan_ip", "255.255.255.255"),
            ]:
                target.write_text(json.dumps({**valid, key: value}))
                with self.assertRaises(ValueError):
                    control.read_config()
            for key in valid:
                target.write_text(
                    json.dumps({k: v for k, v in valid.items() if k != key})
                )
                with self.assertRaises(KeyError):
                    control.read_config()


class LeaseTests(unittest.TestCase):
    def setUp(self):
        self.value = {
            "boot_id": "boot",
            "lease_id": "a" * 32,
            "expires_monotonic": 1000,
        }

    def test_valid_and_invalid(self):
        self.assertTrue(control.valid_lease(self.value, "boot", 100))
        for value in (
            None,
            [],
            {},
            {**self.value, "extra": 1},
            {**self.value, "boot_id": "another"},
            {**self.value, "lease_id": ""},
            {**self.value, "expires_monotonic": True},
            {**self.value, "expires_monotonic": float("nan")},
            {**self.value, "expires_monotonic": float("inf")},
            {**self.value, "expires_monotonic": 1001},
            {**self.value, "expires_monotonic": 100},
        ):
            with self.subTest(value=value):
                self.assertFalse(control.valid_lease(value, "boot", 100))

    def test_atomic_private_write_expiry_wrong_boot_and_malformed_revoke(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "lease"
            with (
                mock.patch.object(control, "boot_id", return_value="boot"),
                mock.patch.object(control.time, "monotonic", return_value=100),
            ):
                value = control.write_lease(path)
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            self.assertEqual(
                set(path.parent.iterdir()), {path, path.with_name("lease.lock")}
            )
            self.assertEqual(
                control.inspect_lease(path, boot="boot", now=101), (value, False)
            )
            self.assertEqual(
                control.inspect_lease(path, boot="boot", now=1000), (None, True)
            )
            self.assertFalse(path.exists())
            for raw in ("bad json", json.dumps(self.value)):
                path.write_text(raw)
                path.chmod(0o600)
                self.assertEqual(
                    control.inspect_lease(path, boot="wrong", now=100), (None, True)
                )
                self.assertFalse(path.exists())
            self.assertEqual(control.inspect_lease(path), (None, False))

    def test_shared_lock_serializes_inspect_write_and_revoke(self):
        with (
            tempfile.TemporaryDirectory() as directory,
            concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool,
        ):
            path = pathlib.Path(directory) / "lease"
            with mock.patch.object(control, "boot_id", return_value="boot"):
                for operation in (
                    control.inspect_lease,
                    control.write_lease,
                    control.revoke_lease,
                ):
                    with self.subTest(operation=operation.__name__):
                        with control.lease_lock(path):
                            future = pool.submit(operation, path)
                            with self.assertRaises(concurrent.futures.TimeoutError):
                                future.result(timeout=0.03)
                        future.result(timeout=2)
                # A renewal waiting behind expiry inspection survives the old removal.
                path.write_text(json.dumps(self.value))
                path.chmod(0o600)
                with control.lease_lock(path):
                    renewal = pool.submit(control.write_lease, path)
                    self.assertEqual(
                        control._inspect_lease_locked(path, boot="boot", now=1000),
                        (None, True),
                    )
                replacement = renewal.result(timeout=2)
                self.assertEqual(json.loads(path.read_text()), replacement)

    def test_exact_keyguard_parser(self):
        for output, expected in (
            ("KeyguardServiceDelegate showing=false", False),
            ("KeyguardServiceDelegate showing=true", True),
            ("KeyguardServiceDelegate showing=false showing=true", None),
            ("KeyguardServiceDelegate showing=false showing=false", None),
            ("KeyguardServiceDelegate showing=falsejunk", None),
            ("KeyguardServiceDelegate showing=unknown", None),
            ("showing=false", None),
            ("KeyguardServiceDelegate KeyguardServiceDelegate showing=false", None),
        ):
            with self.subTest(output=output):
                self.assertIs(control.parse_keyguard_showing(output), expected)


class ActionTests(unittest.TestCase):
    def setUp(self):
        patcher = mock.patch.object(
            control, "read_config", return_value={"serial": "EXAMPLE_SERIAL"}
        )
        patcher.start()
        self.addCleanup(patcher.stop)

    def command(self, locked=False, on=True):
        def answer(*args):
            if args == ("shell", "getprop", "ro.serialno"):
                output = "EXAMPLE_SERIAL"
            elif args == ("shell", "dumpsys", "window", "policy"):
                output = "KeyguardServiceDelegate showing=" + (
                    "true" if locked else "false"
                )
            elif args == ("shell", "dumpsys", "display"):
                output = "mCommittedState=" + ("ON" if on else "OFF")
            else:
                output = ""
            return types.SimpleNamespace(returncode=0, stdout=output)

        return mock.Mock(side_effect=answer)

    def test_start_locked_revokes_existing_lease_and_turns_off(self):
        command = self.command(locked=True)
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "lease"
            path.write_text("old lease")
            with mock.patch.object(control, "_write_lease_locked") as write:
                with self.assertRaises(RuntimeError):
                    control.start_owner(command, path)
                write.assert_not_called()
            self.assertFalse(path.exists())
        self.assertFalse(
            any("power-reset" in call.args for call in command.call_args_list)
        )
        self.assertEqual(command.call_args.args[-2:], ("power-off", "0"))

    def test_start_failure_revokes_and_turns_off(self):
        command = self.command(on=False)
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "lease"
            with (
                mock.patch.object(control, "boot_id", return_value="boot"),
                self.assertRaises(RuntimeError),
            ):
                control.start_owner(command, path)
            self.assertFalse(path.exists())
            self.assertEqual(
                command.call_args.args, ("shell", "cmd", "display", "power-off", "0")
            )

    def test_start_success_stop_idempotent_and_locked_stop_no_chrome(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "lease"
            command = self.command()
            with mock.patch.object(control, "boot_id", return_value="boot"):
                control.start_owner(command, path)
            self.assertTrue(path.exists())
            for _ in range(2):
                control.stop_owner(command, path)
                self.assertFalse(path.exists())
                self.assertEqual(command.call_args.args[-2:], ("power-off", "0"))
            self.assertIn(
                mock.call("shell", "am", "start", "-n", control.CHROME),
                command.call_args_list,
            )
            command = self.command(locked=True)
            control.stop_owner(command, path)
            self.assertFalse(any("am" in call.args for call in command.call_args_list))
            self.assertEqual(command.call_args.args[-2:], ("power-off", "0"))

    def test_start_probe_failure_revokes_and_off(self):
        command = mock.Mock(
            side_effect=[OSError("test"), types.SimpleNamespace(returncode=0)]
        )
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "lease"
            path.write_text("old lease")
            with self.assertRaises(OSError):
                control.start_owner(command, path)
            self.assertFalse(path.exists())
            self.assertEqual(command.call_args.args[-2:], ("power-off", "0"))

    def test_start_transition_excludes_stale_handback_and_watchdog(self):
        base = self.command()
        resetting = threading.Event()
        finish_reset = threading.Event()

        def command(*args):
            if "power-reset" in args:
                resetting.set()
                if not finish_reset.wait(timeout=3):
                    raise RuntimeError("test synchronization timeout")
            return base(*args)

        with (
            tempfile.TemporaryDirectory() as directory,
            concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool,
        ):
            path = pathlib.Path(directory) / "lease"
            with mock.patch.object(control, "boot_id", return_value="boot"):
                starting = pool.submit(control.start_owner, command, path)
                self.assertTrue(resetting.wait(timeout=2))
                old_handback = pool.submit(control.handback_if_unowned, base, path)
                try:
                    with self.assertRaises(concurrent.futures.TimeoutError):
                        old_handback.result(timeout=0.03)
                finally:
                    finish_reset.set()
                starting.result(timeout=2)
                self.assertFalse(old_handback.result(timeout=2))
                self.assertFalse(
                    any(
                        "power-off" in call.args or "am" in call.args
                        for call in base.call_args_list
                    )
                )
                action = mock.Mock()
                self.assertFalse(control.while_unowned(action, path))
                action.assert_not_called()

    def test_failure_status_contains_fixed_code_and_timestamp_only(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "error.json"
            control.record_failure("owner_start_failed", path)
            result = json.loads(path.read_text())
            self.assertEqual(set(result), {"code", "timestamp"})
            self.assertEqual(result["code"], "owner_start_failed")
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            with self.assertRaises(ValueError):
                control.record_failure("secret exception", path)

    def test_handback_always_off_on_probe_failure(self):
        command = mock.Mock(
            side_effect=[OSError("test"), types.SimpleNamespace(returncode=0)]
        )
        with self.assertRaises(OSError):
            control.handback(command)
        self.assertEqual(command.call_args.args[-2:], ("power-off", "0"))


class HTTPTests(unittest.TestCase):
    def handler(self, extra=None):
        handler = object.__new__(control.Handler)
        handler.server = types.SimpleNamespace(
            token="x" * 48, origin="https://192.0.2.116:8443"
        )
        handler.headers = email.message.Message()
        for key, value in {
            "Host": control.HOST,
            "X-Pixel-Control": "x" * 48,
            "Origin": handler.server.origin,
            "Content-Length": "0",
            **(extra or {}),
        }.items():
            if value is not None:
                handler.headers[key] = value
        handler.reply = mock.Mock()
        return handler

    def test_auth_origin_body_and_duplicate_headers(self):
        self.assertTrue(self.handler().authorized(post=True))
        for headers in (
            {"Host": "localhost:8766"},
            {"X-Pixel-Control": "wrong"},
            {"X-Pixel-Control": "ю"},
            {"Origin": "https://evil.invalid"},
            {"Origin": None},
            {"Content-Length": "1"},
            {"Content-Length": "-1"},
            {"Transfer-Encoding": "chunked"},
        ):
            with self.subTest(headers=headers):
                self.assertFalse(self.handler(headers).authorized(post=True))
        for name in ("Host", "X-Pixel-Control", "Origin", "Content-Length"):
            handler = self.handler()
            handler.headers[name] = handler.headers[name]
            self.assertFalse(handler.authorized(post=True))

    def test_landing_links_vnc_only_when_active(self):
        for active in (False, True):
            handler = self.handler()
            handler.path = "/owner"
            with mock.patch.object(
                control,
                "lease_snapshot",
                return_value={"active": active, "remaining": 900 if active else 0},
            ):
                handler.do_GET()
            code, page = handler.reply.call_args.args
            self.assertEqual(code, 200)
            self.assertEqual("href='/vnc.html?autoconnect=true" in page, active)
            self.assertNotIn("<script", page)
            self.assertIn("старый кадр", page)

    def test_status_expiry_is_read_only_and_json(self):
        handler = self.handler()
        handler.path = "/owner/status"
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "lease"
            raw = json.dumps(
                {"boot_id": "boot", "lease_id": "a" * 32, "expires_monotonic": 1000}
            )
            path.write_text(raw)
            path.chmod(0o600)
            with (
                mock.patch.object(control, "LEASE", path),
                mock.patch.object(control, "boot_id", return_value="boot"),
                mock.patch.object(control.time, "monotonic", return_value=1001),
            ):
                handler.do_GET()
            self.assertEqual(path.read_text(), raw)
            code, body, mime = handler.reply.call_args.args
            self.assertEqual((code, mime), (200, "application/json; charset=utf-8"))
            self.assertEqual(json.loads(body), {"active": False, "remaining": 0})
            self.assertFalse(path.with_name("lease.lock").exists())
            with (
                mock.patch.object(control, "LEASE", path),
                mock.patch.object(control, "boot_id", return_value="boot"),
                mock.patch.object(control.time, "monotonic", return_value=101),
            ):
                handler.do_GET()
            self.assertEqual(
                json.loads(handler.reply.call_args.args[1]),
                {"active": True, "remaining": 899},
            )
            self.assertEqual(path.read_text(), raw)

    def test_status_auth_query_and_body_fail_closed(self):
        for path, headers, status in (
            ("/owner/status?x=1", {}, 404),
            ("/owner/status", {"X-Pixel-Control": "wrong"}, 403),
            ("/owner/status", {"Content-Length": "1"}, 403),
        ):
            handler = self.handler(headers)
            handler.path = path
            with mock.patch.object(control, "lease_snapshot") as read:
                handler.do_GET()
                read.assert_not_called()
            self.assertEqual(handler.reply.call_args.args[0], status)

    def test_start_redirects_vnc_and_stop_redirects_owner(self):
        for path, location in (
            ("/owner/start", "/vnc.html?autoconnect=true&resize=scale&path=websockify"),
            ("/owner/stop", "/owner"),
        ):
            handler = self.handler()
            handler.path = path
            handler.send_response = mock.Mock()
            handler.send_header = mock.Mock()
            handler.end_headers = mock.Mock()
            with (
                mock.patch.object(control, "start_owner"),
                mock.patch.object(control, "stop_owner"),
            ):
                handler.do_POST()
            handler.send_response.assert_called_once_with(303)
            self.assertIn(
                mock.call("Location", location), handler.send_header.call_args_list
            )

    def test_junk_paths_and_unauthorized_never_act(self):
        with (
            mock.patch.object(control, "start_owner") as start,
            mock.patch.object(control, "stop_owner") as stop,
        ):
            for path in (
                "/owner/start?x=1",
                "/owner/stop/",
                "/owner/start#junk",
                "/other",
            ):
                handler = self.handler()
                handler.path = path
                handler.do_POST()
                self.assertEqual(handler.reply.call_args.args[0], 404)
            handler = self.handler({"Content-Length": "2"})
            handler.path = "/owner/start"
            handler.do_POST()
            self.assertEqual(handler.reply.call_args.args[0], 403)
            start.assert_not_called()
            stop.assert_not_called()


class OwnerSessionTests(unittest.TestCase):
    def test_javascript_redirects_on_inactive_invalid_and_network_failure(self):
        node = shutil.which("node")
        if node is None:
            self.skipTest("Node.js unavailable for isolated JS regression")
        script = r"""
const fs = require('fs');
const vm = require('vm');
const assert = require('assert/strict');
const source = fs.readFileSync(process.argv[1], 'utf8');
(async () => {
  for (const sample of [false, null, 'error', 'http', 'importerror', true]) {
    const redirects = [], delays = [], imports = [], polls = [];
    const element = () => ({style: {}, append() {}});
    const root = element();
    const context = {
      document: {documentElement: root, body: element(), createElement: element},
      window: {location: {replace: (url) => redirects.push(url)}},
      AbortController,
      setTimeout: (callback, delay) => {delays.push(delay); if (delay === 5000) polls.push(callback); return 1;},
      clearTimeout() {},
      fetch: async (url, options) => {
        assert.equal(url, '/owner/status');
        assert.equal(options.redirect, 'error');
        assert.equal(options.cache, 'no-store');
        if (sample === 'error') throw Error('network failure');
        return {ok: sample !== 'http', json: async () =>
          sample === null ? {} : {active: sample === 'importerror' ? true : sample,
            remaining: sample === true || sample === 'importerror' ? 899 : 0}};
      }
    };
    const sandbox = vm.createContext(context);
    vm.runInContext(source, sandbox, {
      importModuleDynamically: async (specifier) => {
        imports.push(specifier);
        assert.equal(specifier, '/app/ui.js');
        if (sample === 'importerror') throw Error('module unavailable');
        const module = new vm.SyntheticModule([], () => {}, {context: sandbox});
        await module.link(() => {});
        await module.evaluate();
        return module;
      }
    });
    assert.deepEqual(imports, []);
    assert.equal(root.style.visibility, 'hidden');
    await new Promise(resolve => setImmediate(resolve));
    if (sample === true) {
      assert.deepEqual(redirects, []);
      assert.equal(root.style.visibility, 'visible');
      assert.ok(delays.includes(5000));
      assert.deepEqual(imports, ['/app/ui.js']);
      await polls.shift()();
      assert.deepEqual(imports, ['/app/ui.js']);
    } else {
      assert.deepEqual(redirects, ['/owner']);
      assert.equal(root.style.visibility, 'hidden');
      assert.ok(!delays.includes(5000));
      assert.deepEqual(imports, sample === 'importerror' ? ['/app/ui.js'] : []);
    }
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
"""
        result = subprocess.run(
            [
                node,
                "--experimental-vm-modules",
                "-e",
                script,
                str(pathlib.Path(__file__).with_name("owner-session.js")),
            ],
            capture_output=True,
            text=True,
            timeout=10,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_caddy_exact_local_static_precedes_upstream_assets(self):
        source = pathlib.Path(__file__).with_name("Caddyfile.template").read_text()
        self.assertIn("path /vnc.html /owner-session.js", source)
        self.assertIn("root __TERMUX_HOME__/pixel-admin/static", source)
        self.assertLess(
            source.index("file_server @owner_static"),
            source.index("reverse_proxy @assets"),
        )
        self.assertIn("reverse_proxy @owner_status 127.0.0.1:8766", source)


if __name__ == "__main__":
    unittest.main()
