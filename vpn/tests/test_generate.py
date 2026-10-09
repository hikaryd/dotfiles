"""Subscription protocol coverage, credential safety, and manual-node retention."""
import contextlib
import copy
import importlib.util
import io
import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

spec = importlib.util.spec_from_file_location(
    "vpn_generate", Path(__file__).resolve().parents[1] / "generate.py")
generate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(generate)

VLESS = ("vless://00000000-0000-4000-8000-000000000001@vless.example:443"
         "?type=tcp&security=reality&sni=cover.example&pbk=fake-public-key&sid=abcd"
         "&flow=xtls-rprx-vision#Germany%20-%20description")
HYSTERIA2 = "hysteria2://fake-password@hy2.example:8446/?sni=cover.example&insecure=0#Finland"


class ProtocolTests(unittest.TestCase):
    def test_existing_vless_alias_and_reality_fields_are_unchanged(self):
        node = generate.parse_node(VLESS, 6)
        self.assertEqual(node["tag"], "Germany #6")
        self.assertEqual(node["type"], "vless")
        self.assertEqual(node["server_port"], 443)
        self.assertEqual(node["flow"], "xtls-rprx-vision")
        self.assertEqual(node["tls"]["reality"], {
            "enabled": True, "public_key": "fake-public-key", "short_id": "abcd"})
        self.assertNotIn("transport", node)

    def test_grpc_uses_the_subscription_service_name(self):
        node = generate.parse_node(VLESS.replace("type=tcp", "type=grpc&serviceName=stream"), 2)
        self.assertEqual(node["transport"], {"type": "grpc", "service_name": "stream"})

    def test_hysteria2_tls_and_udp_port_do_not_set_bandwidth_or_network_limits(self):
        node = generate.parse_node(HYSTERIA2, 9)
        self.assertEqual(node["tag"], "Finland #9")
        self.assertEqual(node["type"], "hysteria2")
        self.assertEqual(node["server_port"], 8446)
        self.assertEqual(node["password"], "fake-password")
        self.assertEqual(node["tls"], {"enabled": True, "server_name": "cover.example"})
        for key in ("network", "up_mbps", "down_mbps", "transport", "uuid"):
            self.assertNotIn(key, node)

    def test_hy2_alias_ipv6_and_percent_encoded_auth_preserve_special_characters(self):
        uri = ("hy2://fake%3Apass%2Bword%40%2F%3F%23%25@[2001:db8::1]:8446/"
               "?sni=cover%2Eexample&insecure=1#Finland%20Gaming")
        node = generate.parse_node(uri, 9)
        self.assertEqual(node["server"], "2001:db8::1")
        self.assertEqual(node["password"], "fake:pass+word@/?#%")
        self.assertEqual(node["tls"]["server_name"], "cover.example")
        self.assertTrue(node["tls"]["insecure"])
        self.assertEqual(node["tag"], "Finland Gaming #9")

    def test_userpass_auth_and_default_port(self):
        node = generate.parse_node("hysteria2://fake-user:fake+password@hy2.example", 3)
        self.assertEqual(node["password"], "fake-user:fake+password")
        self.assertEqual(node["server_port"], 443)
        self.assertEqual(node["tag"], "node3 #3")
        ipv6 = generate.parse_node("hy2://fake-password@[2001:db8::1]", 4)
        self.assertEqual(ipv6["server_port"], 443)

    def test_salamander_password_is_decoded(self):
        node = generate.parse_node(
            HYSTERIA2.replace("insecure=0", "obfs=salamander&obfs-password=fake%2Bpass%26word"), 9)
        self.assertEqual(node["obfs"], {"type": "salamander", "password": "fake+pass&word"})

    def test_multi_port_addresses_map_to_sing_box_ranges_without_single_port(self):
        for host in ("hy2.example", "[2001:db8::1]"):
            with self.subTest(host=host):
                node = generate.parse_node(f"hy2://fake-password@{host}:443,5000-6000,8446/", 9)
                self.assertEqual(node["server_ports"], ["443", "5000:6000", "8446"])
                self.assertNotIn("server_port", node)

    def test_missing_or_invalid_endpoint_auth_obfs_and_tls_are_safely_rejected(self):
        uris = [
            "hy2://hy2.example:443", "hy2://@hy2.example:443",
            "hy2://fake-password@:443", "hy2://fake-password@[invalid]:443",
            "hy2://fake-password@hy2.example:65536", "hy2://fake-password@hy2.example:0",
            "hy2://fake-password@hy2.example:5000-4000",
            "hy2://fake-password@hy2.example:443,,8446",
            "hy2://fake-password@[2001:db8::1]8446",
            "hy2://fake-password@hy2.example:443/extra",
            "hy2://fake-password@hy2.example?insecure=fake-password",
            "hy2://fake-password@hy2.example?obfs=salamander",
            "hy2://fake-password@hy2.example?obfs=fake-password&obfs-password=fake-password",
            "hy2://fake-password@hy2.example?obfs-password=fake-password",
            "hy2://fake-password@hy2.example?pinSHA256=fake-password",
            "hy2://fake-password@hy2.example?ech=fake-password",
            "hy2://fake%FFpassword@hy2.example",
            VLESS.replace(":443?", ":99999?"),
        ]
        for uri in uris:
            with self.subTest(uri=uri), self.assertRaises(generate.NodeParseError) as raised:
                generate.parse_node(uri, 9)
            self.assertNotIn("fake-password", str(raised.exception))
            self.assertNotIn(uri, str(raised.exception))

    def test_unknown_protocol_and_transport_do_not_expose_source_credentials(self):
        source = "\n".join([
            "trojan://fake-password@secret.example:443#secret-name",
            VLESS.replace("type=tcp", "type=xhttp"),
            VLESS.replace("type=tcp", "type=fake-password"),
            HYSTERIA2,
        ])
        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr):
            nodes = generate.parse_subscription(source)
        self.assertEqual([node["tag"] for node in nodes], ["Finland #3"])
        log = stderr.getvalue()
        self.assertIn("skip line 0: unsupported protocol", log)
        self.assertIn("skip line 1: xhttp transport is not supported by sing-box", log)
        self.assertIn("skip line 2: unsupported VLESS transport", log)
        for secret in ("fake-password", "00000000-0000-4000-8000-000000000001", "secret.example"):
            self.assertNotIn(secret, log)


class RetentionTests(unittest.TestCase):
    def preferred_nodes(self):
        return generate.parse_subscription("\n".join([
            VLESS,
            HYSTERIA2.replace("#Finland", "#Hysteria%20Finland"),
            VLESS.replace("vless.example", "estonia.example").replace("#Germany", "#Estonia"),
            HYSTERIA2.replace("hy2.example", "other-hy2.example").replace("#Finland", "#Hysteria%20Other"),
        ]))

    def test_preference_is_stable_and_applies_before_cap_with_compatible_default(self):
        nodes = self.preferred_nodes()
        original = copy.deepcopy(nodes)
        self.assertEqual(generate.auto_pool(nodes, "", "", 2), ["Germany #0", "Hysteria Finland #1"])
        self.assertEqual(generate.auto_pool(nodes, "", "", 2, "hYsTeRiA"),
                         ["Hysteria Finland #1", "Hysteria Other #3"])
        self.assertEqual(generate.auto_pool(nodes, "", "", 4, "Hysteria"),
                         ["Hysteria Finland #1", "Hysteria Other #3", "Germany #0", "Estonia #2"])
        self.assertEqual(generate.auto_pool(nodes, "", "", 2, "no-match"),
                         ["Germany #0", "Hysteria Finland #1"])
        self.assertEqual(nodes, original)

    def test_preference_never_bypasses_include_or_exclude(self):
        nodes = self.preferred_nodes()
        self.assertEqual(generate.auto_pool(nodes, "", "Hysteria", 2, "Hysteria"),
                         ["Germany #0", "Estonia #2"])
        self.assertEqual(generate.auto_pool(nodes, "Estonia", "", 2, "Hysteria"), ["Estonia #2"])

    def test_dedup_retains_preferred_alias_without_removing_manual_nodes(self):
        nodes = generate.parse_subscription(f"{VLESS}\n{VLESS.replace('#Germany', '#Preferred')}")
        self.assertEqual(generate.auto_pool(nodes, "", "", 2, "Preferred"), ["Preferred #1"])
        self.assertEqual([node["tag"] for node in nodes], ["Germany #0", "Preferred #1"])

    def test_invalid_preference_preserves_existing_config_without_config_check(self):
        with tempfile.TemporaryDirectory() as directory:
            config = Path(directory) / "config.json"
            config.write_text("previous valid config", encoding="utf-8")
            with patch.multiple(generate, CFG=str(config),
                                SUB_URL="https://subscription.example/fake-token",
                                AUTO_INCLUDE="", AUTO_EXCLUDE="", AUTO_MAX=1,
                                AUTO_PREFER="["), patch.object(generate, "ensure_srs"), patch.object(
                    generate, "fetch", return_value=HYSTERIA2), patch.object(
                    generate.subprocess, "run") as check, self.assertRaises(ValueError) as raised:
                generate.main()
            self.assertIn("AUTO_PREFER", str(raised.exception))
            self.assertEqual(config.read_text(encoding="utf-8"), "previous valid config")
            self.assertFalse(Path(str(config) + ".tmp").exists())
            check.assert_not_called()

    def test_auto_filter_cap_and_dedup_preserve_every_manual_alias_and_protocol(self):
        nodes = generate.parse_subscription(f"{VLESS}\n{HYSTERIA2}\n{VLESS}\n{HYSTERIA2}")
        original = copy.deepcopy(nodes)
        self.assertEqual([node["tag"] for node in nodes],
                         ["Germany #0", "Finland #1", "Germany #2", "Finland #3"])
        self.assertEqual(generate.auto_pool(nodes, "", "", 8), ["Germany #0", "Finland #1"])
        self.assertEqual(generate.auto_pool(nodes, "", "Finland", 1), ["Germany #0"])
        self.assertEqual(nodes, original)
        for include, exclude, limit in [("[", "", 1), ("", ".*", 1), ("", "", 0)]:
            with self.subTest(include=include, exclude=exclude, limit=limit), self.assertRaises(ValueError):
                generate.auto_pool(nodes, include, exclude, limit)
        self.assertEqual(nodes, original)

    def test_generated_selector_keeps_all_nodes_even_when_auto_pool_is_capped(self):
        with tempfile.TemporaryDirectory() as directory:
            config = Path(directory) / "config.json"
            secret = Path(directory) / "secret.txt"
            secret.write_text("fake-panel-secret", encoding="utf-8")
            source = f"{VLESS}\n{HYSTERIA2}\n{VLESS}"
            with patch.multiple(generate, CFG=str(config), SECRET_FILE=str(secret),
                                SUB_URL="https://subscription.example/fake-token",
                                AUTO_INCLUDE="", AUTO_EXCLUDE="Finland", AUTO_MAX=1), patch.object(
                    generate, "ensure_srs"), patch.object(generate, "fetch", return_value=source), patch.object(
                    generate.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, "", "")), \
                    contextlib.redirect_stdout(io.StringIO()):
                generate.main()
            outbounds = json.loads(config.read_text(encoding="utf-8"))["outbounds"]
            selector, auto = outbounds[:2]
            self.assertEqual(selector["default"], "auto")
            self.assertEqual(selector["outbounds"], ["auto", "Germany #0", "Finland #1", "Germany #2"])
            self.assertEqual(auto["outbounds"], ["Germany #0"])
            self.assertEqual(outbounds[3]["type"], "hysteria2")

    def test_config_check_failure_preserves_previous_config_and_hides_authentication(self):
        with tempfile.TemporaryDirectory() as directory:
            config = Path(directory) / "config.json"
            config.write_text("previous valid config", encoding="utf-8")
            secret = Path(directory) / "secret.txt"
            secret.write_text("fake-panel-secret", encoding="utf-8")
            check = subprocess.CompletedProcess([], 1, "", "invalid auth fake-password fake-panel-secret")
            with patch.multiple(generate, CFG=str(config), SECRET_FILE=str(secret),
                                SUB_URL="https://subscription.example/fake-token",
                                AUTO_INCLUDE="", AUTO_EXCLUDE="", AUTO_MAX=1), patch.object(
                    generate, "ensure_srs"), patch.object(generate, "fetch", return_value=HYSTERIA2), patch.object(
                    generate.subprocess, "run", return_value=check), self.assertRaises(SystemExit) as raised:
                generate.main()
            self.assertEqual(config.read_text(encoding="utf-8"), "previous valid config")
            self.assertIn("CONFIG CHECK FAILED", str(raised.exception))
            self.assertNotIn("fake-password", str(raised.exception))
            self.assertNotIn("fake-panel-secret", str(raised.exception))


if __name__ == "__main__":
    unittest.main()
