"""Проверки LAN-политики и обработки ошибок HTTP RCI без реального роутера."""

import importlib.util
import io
import json
import os
import unittest
import urllib.error
from pathlib import Path
from unittest.mock import patch

SCRIPT = Path(__file__).resolve().parents[1] / "vpn/router-setup.py"


class RouterTests(unittest.TestCase):
    def setUp(self):
        self.env = patch.dict(os.environ, {
            "ROUTER_PASS": "test-password", "ROUTER_LAN_INTERFACE": "Home",
            "SERVER_HOST": "192.0.2.10", "SERVER_PUB": "test-public-key",
            "ROUTER_KEY": "test-private-key", "AWG_PARAMS": "3 10 30 20 30 100 200 300 400",
        })
        self.env.start()
        self.addCleanup(self.env.stop)
        spec = importlib.util.spec_from_file_location("vpn_router", SCRIPT)
        self.router = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.router)
        self.inventory = {
            "Bridge0": {"id": "Bridge0", "interface-name": "Home", "global": False},
            "GigabitEthernet1": {
                "id": "GigabitEthernet1", "interface-name": "ISP", "global": True,
            },
            "Wireguard0": {"id": "Wireguard0", "global": True},
            "GigabitEthernet0": {"id": "GigabitEthernet0"},
        }

    def interface_response(self, path, data=None, method=None, hdr=None):
        if path == "/rci/show/interface":
            return io.BytesIO(json.dumps(self.inventory).encode())
        if path == "/rci/show" and method == "POST":
            interfaces = {
                "Home": {"id": "Bridge0", "interface-name": "Home"},
                "Bridge1": {"id": "Bridge1", "interface-name": "Guest"},
                "Wireguard0": {"id": "Wireguard0", "wireguard": {"public-key": "test-public-key"}},
            }
            name = json.loads(data)[0]["interface"]["name"]
            return io.BytesIO(json.dumps([{"interface": interfaces.get(name, {})}]).encode())
        if path == "/rci/show/ip/route":
            return io.BytesIO(b"[]")
        raise urllib.error.HTTPError(path, 404, "Not Found", {}, None)

    def test_router_connection_ignores_system_and_environment_proxies(self):
        with patch.object(self.router.urllib.request, "getproxies", return_value={
            "http": "http://127.0.0.1:10810", "https": "http://127.0.0.1:10810",
        }), patch.object(self.router.urllib.request, "build_opener", wraps=
                       self.router.urllib.request.build_opener) as build:
            spec = importlib.util.spec_from_file_location("vpn_router_proxy_test", SCRIPT)
            router = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(router)
        proxy_handlers = [handler for handler in build.call_args.args
                          if isinstance(handler, self.router.urllib.request.ProxyHandler)]
        self.assertEqual(len(proxy_handlers), 1)
        self.assertEqual(proxy_handlers[0].proxies, {})
        self.assertFalse(any(isinstance(handler, self.router.urllib.request.ProxyHandler)
                             for handler in router.op.handlers))

    def test_interface_alias_uses_post_not_nested_url(self):
        with patch.object(self.router, "raw", side_effect=self.interface_response) as raw:
            result = self.router.get_interface("Home")
        self.assertEqual(result["id"], "Bridge0")
        raw.assert_called_once_with(
            "/rci/show", json.dumps([{"interface": {"name": "Home"}}]).encode(),
            "POST", {"Content-Type": "application/json"},
        )

    def test_interface_http_failure_is_not_treated_as_missing(self):
        error = urllib.error.HTTPError("/rci/show/interface", 401, "Unauthorized", {}, None)
        self.addCleanup(error.close)
        with patch.object(self.router, "raw", side_effect=error), \
                self.assertRaises(urllib.error.HTTPError):
            self.router.get_interface("Home")

    def test_interface_rejects_malformed_or_wrong_response(self):
        responses = [
            {}, [], [{}], [{"interface": []}],
            [{"interface": {"id": "Bridge1", "interface-name": "Guest"}}],
        ]
        for response in responses:
            with self.subTest(response=response), \
                    patch.object(self.router, "raw", return_value=io.BytesIO(json.dumps(response).encode())), \
                    self.assertRaises(RuntimeError):
                self.router.get_interface("Home")

    def test_identify_reads_current_wireguard_key_via_post(self):
        output = io.StringIO()
        with patch.object(self.router, "auth"), \
                patch.object(self.router, "get", return_value={"hostname": "Keenetic-4508"}), \
                patch.object(self.router, "raw", side_effect=self.interface_response), \
                patch("sys.stdout", output):
            self.router.identify()
        self.assertIn("NAME=Keenetic-4508", output.getvalue())
        self.assertIn("CURPUB=test-public-key", output.getvalue())

    def test_install_assigns_native_policy_before_save(self):
        with patch.object(self.router, "auth"), patch.object(self.router, "run") as run, \
                patch.object(self.router, "raw", side_effect=self.interface_response), \
                patch.object(self.router.time, "sleep"):
            self.router.install()
        commands = [cmd for call in run.call_args_list for cmd in call.args[0]]
        self.assertIn("ip policy AWG_LAN", commands)
        self.assertIn("permit global Wireguard0", commands)
        self.assertIn("no permit auto", commands)
        self.assertNotIn("no permit", commands)
        self.assertLess(commands.index("no permit auto"), commands.index("permit global Wireguard0"))
        self.assertLess(commands.index("permit global Wireguard0"),
                        commands.index("no permit global GigabitEthernet1"))
        binding = "ip hotspot policy Home AWG_LAN"
        self.assertIn(binding, commands)
        self.assertLess(commands.index(binding), commands.index("system configuration save"))
        self.assertFalse(any(cmd.startswith("permit global") and "Wireguard0" not in cmd
                             for cmd in commands))
        self.assertNotIn("no permit global Bridge0", commands)
        self.assertNotIn("no permit global Wireguard0", commands)
        self.assertFalse(any("Policy0" in cmd for cmd in commands))

    def test_policy_excludes_additional_global_vlan_interface(self):
        name = "GigabitEthernet0/Vlan100"
        self.inventory[name] = {"id": name, "global": True}
        with patch.object(self.router, "auth"), patch.object(self.router, "run") as run, \
                patch.object(self.router, "raw", side_effect=self.interface_response), \
                patch.object(self.router.time, "sleep"):
            self.router.install()
        commands = [cmd for call in run.call_args_list for cmd in call.args[0]]
        self.assertIn(f"no permit global {name}", commands)
        self.assertFalse(any(cmd.startswith(f"interface {name}") for cmd in commands))

    def test_invalid_inventory_stops_before_any_configuration_commands(self):
        cases = [None, [], {}, {"WAN": None}, {"WAN": {"global": True}},
                 {"WAN": {"id": "WAN", "global": "true"}},
                 {"WAN;exit": {"id": "WAN;exit", "global": True}}]
        for inventory in cases:
            self.inventory = inventory
            with self.subTest(inventory=inventory), patch.object(self.router, "auth"), \
                    patch.object(self.router, "run") as run, \
                    patch.object(self.router, "raw", side_effect=self.interface_response), \
                    self.assertRaises(RuntimeError):
                self.router.install()
            run.assert_not_called()

    def test_inventory_uses_ids_not_numeric_port_aliases(self):
        self.inventory["1"] = {"id": "GigabitEthernet0/0", "interface-name": "1"}
        self.inventory["ISP"] = self.inventory.pop("GigabitEthernet1")
        with patch.object(self.router, "raw", side_effect=self.interface_response):
            self.assertEqual(self.router.get_global_interfaces(), ["Wireguard0", "GigabitEthernet1"])

    def test_missing_lan_stops_before_configuration_changes(self):
        with patch.object(self.router, "LAN_IFACE", "Missing"), \
                patch.object(self.router, "auth"), patch.object(self.router, "run") as run, \
                patch.object(self.router, "raw", side_effect=self.interface_response), \
                self.assertRaisesRegex(RuntimeError, "LAN"):
            self.router.install()
        run.assert_not_called()

    def test_custom_lan_interface_is_checked_and_bound(self):
        with patch.object(self.router, "LAN_IFACE", "Bridge1"), \
                patch.object(self.router, "auth"), patch.object(self.router, "run") as run, \
                patch.object(self.router, "raw", side_effect=self.interface_response) as raw, \
                patch.object(self.router.time, "sleep"):
            self.router.install()
        raw.assert_any_call(
            "/rci/show", json.dumps([{"interface": {"name": "Bridge1"}}]).encode(),
            "POST", {"Content-Type": "application/json"},
        )
        commands = [cmd for call in run.call_args_list for cmd in call.args[0]]
        self.assertIn("ip hotspot policy Bridge1 AWG_LAN", commands)

    def test_batch_rejects_command_errors(self):
        response = [{"parse": {"status": [{"status": "error", "message": "unsupported"}]}}]
        with patch.object(self.router, "raw", return_value=io.BytesIO(json.dumps(response).encode())), \
                self.assertRaisesRegex(RuntimeError, "unsupported"):
            self.router.batch(["ip policy AWG_LAN"])

    def test_dns_cleanup_accepts_only_confirmed_missing_server_error(self):
        for address in ("8.8.8.8", "77.88.8.8"):
            response = [{"parse": {"status": [{
                "status": "error", "code": "22544585", "ident": "Dns::Manager",
                "message": f"no such server: {address}.",
            }]}}]
            command = f"no ip name-server {address}"
            with self.subTest(address=address), patch.object(self.router, "raw", return_value=
                    io.BytesIO(json.dumps(response).encode())):
                self.assertEqual(self.router.batch([command]), [(command, "уже отсутствует")])

    def test_dns_cleanup_does_not_hide_other_errors(self):
        status = {"status": "error", "code": "22544585", "ident": "Dns::Manager",
                  "message": "no such server: 8.8.8.8."}
        cases = [
            ("ip name-server 8.8.8.8", [status]),
            ("no ip name-server 9.9.9.9", [status]),
            ("no ip name-server 77.88.8.8", [status]),
            ("no ip name-server 8.8.8.8", [dict(status, code="other")]),
            ("no ip name-server 8.8.8.8", [dict(status, ident="Other")]),
            ("no ip name-server 8.8.8.8", [status, dict(status, message="permission denied")]),
        ]
        for command, statuses in cases:
            response = [{"parse": {"status": statuses}}]
            with self.subTest(command=command, statuses=statuses), \
                    patch.object(self.router, "raw", return_value=io.BytesIO(
                        json.dumps(response).encode())), self.assertRaises(RuntimeError):
                self.router.batch([command])

    def test_batch_rejects_missing_command_results(self):
        with patch.object(self.router, "raw", return_value=io.BytesIO(b"[]")), \
                self.assertRaises(RuntimeError):
            self.router.batch(["ip policy AWG_LAN"])

    def test_batch_rejects_missing_status(self):
        with patch.object(self.router, "raw", return_value=io.BytesIO(b'[{"parse": {}}]')), \
                self.assertRaises(RuntimeError):
            self.router.batch(["ip policy AWG_LAN"])

    def test_batch_accepts_observed_wireguard_peer_context_response(self):
        commands = ["interface Wireguard0", "wireguard peer test-public-key", "exit", "exit"]
        done = [{"status": "message", "code": "1179652",
                 "ident": "Core::Configurator", "message": "done."}]
        response = [
            {"parse": {"prompt": "(config-if)", "status": [
                {"status": "message", "code": "1179653",
                 "ident": "Core::Configurator", "message": "done."}]}},
            {"parse": {"prompt": "(config-wg-peer)"}},
            {"parse": {"prompt": "(config-if)", "status": done}},
            {"parse": {"prompt": "(config)", "status": done}},
        ]
        with patch.object(self.router, "raw", return_value=io.BytesIO(json.dumps(response).encode())):
            result = self.router.batch(commands)
        self.assertEqual(result[1], (commands[1], "(config-wg-peer)"))

    def test_batch_rejects_unconfirmed_context_and_malformed_status(self):
        cases = [
            ("wireguard peer test-public-key", {"prompt": "(config-if)"}),
            ("wireguard peer test-public-key", {"prompt": "(config-wg-peer)", "status": []}),
            ("wireguard peer test-public-key", {"prompt": "(config-wg-peer)", "unexpected": True}),
            ("endpoint 192.0.2.1:51820", {"prompt": "(config-wg-peer)"}),
            ("wireguard peer test-public-key", {"status": [None]}),
            ("wireguard peer test-public-key", None),
        ]
        for command, parse in cases:
            with self.subTest(command=command, parse=parse), \
                    patch.object(self.router, "raw", return_value=io.BytesIO(
                        json.dumps([{"parse": parse}]).encode())), self.assertRaises(RuntimeError):
                self.router.batch([command])

    def test_run_redacts_private_key_in_command_and_response(self):
        key = "different-private-key="
        command = f"interface Wireguard0 wireguard private-key {key}"
        response = [{"parse": {"status": [{"status": "message", "message": f"saved {key}"}]}}]
        output = io.StringIO()
        with patch.object(self.router, "raw", return_value=io.BytesIO(json.dumps(response).encode())), \
                patch("sys.stdout", output):
            self.router.run([command], "Ключ")
        self.assertNotIn(key, output.getvalue())
        self.assertIn("[REDACTED]", output.getvalue())

    def test_batch_redacts_private_key_in_errors_from_other_commands(self):
        key = "different-private-key="
        commands = [f"interface Wireguard0 wireguard private-key {key}", "exit"]
        response = [
            {"parse": {"status": [{"status": "message", "message": "saved"}]}},
            {"parse": {"status": [{"status": "error", "message": f"bad key {key}"}]}},
        ]
        with patch.object(self.router, "raw", return_value=io.BytesIO(json.dumps(response).encode())), \
                self.assertRaises(RuntimeError) as error:
            self.router.batch(commands)
        self.assertNotIn(key, str(error.exception))
        self.assertIn("[REDACTED]", str(error.exception))

    def test_batch_posts_parse_commands_over_http(self):
        response = [{"parse": {"status": [{"status": "message", "message": "ok"}]}}]
        with patch.object(self.router, "raw", return_value=io.BytesIO(json.dumps(response).encode())) as raw:
            self.assertEqual(self.router.batch(["ip policy AWG_LAN"]), [("ip policy AWG_LAN", "ok")])
        args = raw.call_args.args
        self.assertEqual(args[0], "/rci/")
        self.assertEqual(json.loads(args[1]), [{"parse": "ip policy AWG_LAN"}])
        self.assertEqual(args[2], "POST")


if __name__ == "__main__":
    unittest.main()
