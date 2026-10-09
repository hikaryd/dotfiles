import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("tcp_tuning", Path(__file__).parents[1] / "tcp-tuning.py")
tuning = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(tuning)

# Unfiltered VPS iproute2 JSON includes dev, but represents /32 destinations as bare IPs.
# Additional routes exercise diagnostic/foreign markers and an unknown interface.
GLOBAL_ROUTE_JSON = """[
  {"dst":"10.8.2.0/24","dev":"awg0","protocol":"kernel","scope":"link","prefsrc":"10.8.2.1","flags":[]},
  {"dst":"10.8.2.2","dev":"awg0","protocol":"static","metric":42705,"flags":[],"metrics":[{"congestion":"bbr"}]},
  {"dst":"10.8.2.3","dev":"awg0","protocol":"static","metric":42704,"flags":[],"metrics":[{"congestion":"bbr"}]},
  {"dst":"10.8.2.4","dev":"awg0","protocol":"static","metric":20,"flags":[],"metrics":[{"congestion":"bbr"}]},
  {"dst":"10.8.2.5","dev":"eth0","protocol":"static","metric":42705,"metrics":[{"congestion":"bbr"}]},
  {"dst":"10.8.2.6","protocol":"static","metric":42705,"metrics":[{"congestion":"bbr"}]}
]"""


def host_route(peer, metric=42705, congestion="bbr", device="awg0", protocol="static"):
    return {"dst": peer, "dev": device, "protocol": protocol, "metric": metric,
            "metrics": [{"congestion": congestion}]}


class Network:
    """Stateful command boundary: emulate kernel add/del and catch foreign deletion."""

    def __init__(self, routes=None):
        self.routes = list(routes or [])
        self.commands = []
        self.fail_add = None

    def run(self, *args):
        self.commands.append(args)
        if args[0] == "awg":
            return "PRIVATE-PUBLIC-KEY\t10.8.2.2/32\nOTHER-KEY\t10.8.2.3/32\n"
        if args[:5] == ("ip", "-j", "-4", "route", "show"):
            return json.dumps(self.routes)
        if args[:4] == ("ip", "-4", "route", "add"):
            if args[4] == self.fail_add:
                raise tuning.TuningError("simulated route failure")
            self.routes.append(host_route(args[4]))
        elif args[:4] == ("ip", "-4", "route", "del"):
            matches = [r for r in self.routes if tuning.destination(r) == args[4] and tuning.owned(r)]
            if len(matches) != 1:
                raise AssertionError("Attempt to delete a foreign or missing route")
            self.routes.remove(matches[0])
        elif args[:5] == ("ip", "-4", "route", "show", "exact"):
            return f"{args[5]} dev awg0 proto static metric 42705 congctl lock bbr\n"
        return ""


class TcpTuningTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        directory = Path(self.temp.name)
        self.config = directory / "config.json"
        self.helper = directory / "installed.py"
        self.unit = directory / "unit.service"
        self.patcher = patch.multiple(tuning, CONFIG=self.config, HELPER=self.helper, UNIT_PATH=self.unit)
        self.patcher.start()
        self.addCleanup(self.patcher.stop)

    def config_peers(self, *peers):
        self.config.write_text(json.dumps({"peers": peers}))

    def test_apply_only_explicit_registered_peer_preserves_second_home_and_globals(self):
        self.config_peers("10.8.2.2/32")
        other = host_route("10.8.2.3/32", metric=42703, congestion="cubic")
        network = Network([other, {"dst": "10.8.2.0/24", "dev": "awg0", "protocol": "kernel"}])
        with patch.object(tuning, "run", network.run):
            tuning.apply()
        self.assertIn(other, network.routes)
        self.assertTrue(any(tuning.owned(r) and r["dst"] == "10.8.2.2/32" for r in network.routes))
        added = next(c for c in network.commands if c[:4] == ("ip", "-4", "route", "add"))
        self.assertEqual(added[-3:], ("congctl", "lock", "bbr"))
        self.assertFalse(any(c[0] in ("sysctl", "tc", "systemctl") for c in network.commands))

    def test_unregistered_or_subnet_peer_fails_before_any_mutation(self):
        for peer in ("10.8.2.99/32", "10.8.2.0/24", "::1/128", 2):
            with self.subTest(peer=peer):
                self.config_peers(peer)
                network = Network()
                with patch.object(tuning, "run", network.run), self.assertRaises(tuning.TuningError):
                    tuning.apply()
                self.assertFalse(any(c[0] in ("modprobe", "systemctl") or "add" in c for c in network.commands))

    def test_foreign_host_route_on_any_device_is_never_overwritten(self):
        self.config_peers("10.8.2.2/32")
        for route in (host_route("10.8.2.2/32", metric=20), host_route("10.8.2.2/32", device="eth0")):
            network = Network([route])
            with patch.object(tuning, "run", network.run), self.assertRaises(tuning.TuningError):
                tuning.apply()
            self.assertEqual(network.routes, [route])
            self.assertFalse(any("add" in c or "del" in c or c[0] == "modprobe" for c in network.commands))

    def test_cleanup_requires_every_ownership_marker_and_needs_no_config(self):
        own = host_route("10.8.2.2/32")
        foreign = [host_route("10.8.2.3/32", metric=42703),
                   host_route("10.8.2.7/32", metric=42704),
                   host_route("10.8.2.4/32", congestion="cubic"),
                   host_route("10.8.2.5/32", protocol="boot"),
                   host_route("10.8.2.6/32", device="eth0")]
        network = Network([own, *foreign])
        with patch.object(tuning, "run", network.run):
            tuning.cleanup()
        self.assertEqual(network.routes, foreign)

    def test_add_failure_rolls_back_new_routes_and_keeps_existing_owned_route(self):
        self.config_peers("10.8.2.2/32", "10.8.2.3/32")
        previous = host_route("10.8.2.9/32")
        network = Network([previous])
        network.fail_add = "10.8.2.3/32"
        with patch.object(tuning, "run", network.run), self.assertRaises(tuning.TuningError):
            tuning.apply()
        self.assertEqual(network.routes, [previous])

    def test_reapply_is_idempotent_and_removes_only_deselected_owned_route(self):
        self.config_peers("10.8.2.2/32")
        network = Network([host_route("10.8.2.2/32"), host_route("10.8.2.3/32")])
        with patch.object(tuning, "run", network.run):
            tuning.apply()
            tuning.apply()
        self.assertEqual([r["dst"] for r in network.routes], ["10.8.2.2/32"])
        self.assertFalse(any("add" in c for c in network.commands))

    def global_json_network(self):
        return Network(json.loads(GLOBAL_ROUTE_JSON))

    def test_actual_global_json_with_bare_host_destination_is_idempotent(self):
        self.config_peers("10.8.2.2/32")
        network = self.global_json_network()
        before = list(network.routes)
        with patch.object(tuning, "run", network.run):
            tuning.apply()
            tuning.apply()
        self.assertEqual(network.routes, before)
        self.assertFalse(any("add" in c or "del" in c for c in network.commands))

    def test_global_json_cleanup_preserves_diagnostic_foreign_and_missing_dev_routes(self):
        network = self.global_json_network()
        foreign = [route for route in network.routes if route["dst"] != "10.8.2.2"]
        with patch.object(tuning, "run", network.run):
            unknown = next(route for route in tuning.routes() if route["dst"] == "10.8.2.6")
            self.assertNotIn("dev", unknown)
            self.assertFalse(tuning.owned(unknown))
            tuning.cleanup()
        self.assertEqual(network.routes, foreign)
        self.assertEqual([c[4] for c in network.commands if c[:4] == ("ip", "-4", "route", "del")],
                         ["10.8.2.2/32"])

    def test_install_default_disabled_reinstall_preserves_opt_in_and_no_awg_restart(self):
        network = Network()
        with patch.object(tuning, "run", network.run):
            tuning.install()
            self.assertEqual(json.loads(self.config.read_text()), {"peers": []})
            tuning.install(["10.8.2.2/32"])
            content = self.config.read_bytes()
            tuning.install()
        self.assertEqual(self.config.read_bytes(), content)
        self.assertEqual(self.config.stat().st_mode & 0o777, 0o600)
        self.assertIn(f"BindsTo={tuning.PARENT}", self.unit.read_text())
        self.assertIn(f"PartOf={tuning.PARENT}", self.unit.read_text())
        self.assertIn(f"WantedBy={tuning.PARENT}", self.unit.read_text())
        self.assertIn(" cleanup", self.unit.read_text())
        self.assertEqual([c for c in network.commands if "restart" in c],
                         [("systemctl", "restart", tuning.SERVICE)] * 3)

    def test_inactive_awg_install_fails_before_writing(self):
        with patch.object(tuning, "run", side_effect=tuning.TuningError("inactive")), self.assertRaises(tuning.TuningError):
            tuning.install()
        self.assertFalse(self.config.exists() or self.helper.exists() or self.unit.exists())

    def test_disable_cleans_own_route_and_clears_opt_in_for_future_reinstall(self):
        self.config_peers("10.8.2.2/32")
        network = Network([host_route("10.8.2.2/32")])
        with patch.object(tuning, "run", network.run):
            tuning.disable()
        self.assertEqual(network.routes, [])
        self.assertEqual(json.loads(self.config.read_text()), {"peers": []})

    def test_failed_real_child_process_does_not_expose_captured_output(self):
        with self.assertRaises(tuning.TuningError) as failure:
            tuning.run(sys.executable, "-c", "import sys; print('fake-private-output'); sys.exit(7)")
        self.assertNotIn("fake-private-output", str(failure.exception))
        self.assertIn("exit 7", str(failure.exception))


if __name__ == "__main__":
    unittest.main()
