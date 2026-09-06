"""Регрессии генерации VPN-конфига без сети и доступа к VPS."""

import importlib.util
import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

GENERATOR = Path(__file__).resolve().parents[1] / "vpn/generate.py"


class GenerateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        root = Path(self.temp.name)
        with patch.dict(os.environ, {"VPN_ENV": str(root / "vpn.env")}):
            spec = importlib.util.spec_from_file_location("vpn_generate", GENERATOR)
            self.generator = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(self.generator)
        self.generator.SUB_URL = "https://example.invalid/sub"
        self.generator.CFG = str(root / "config.json")
        self.generator.SECRET_FILE = str(root / "secret.txt")
        Path(self.generator.SECRET_FILE).write_text("test-secret")
        for target, kwargs in (
            ("ensure_srs", {}),
            ("fetch", {"return_value": "vless://test@192.0.2.1:443#test"}),
        ):
            mock = patch.object(self.generator, target, **kwargs)
            mock.start()
            self.addCleanup(mock.stop)

    def test_domain_uses_proxy_before_direct_and_adblock(self):
        for adblock in (True, False):
            with self.subTest(adblock=adblock):
                self.generator.ADBLOCK = adblock
                with patch.object(self.generator.subprocess, "run") as check:
                    check.return_value = subprocess.CompletedProcess([], 0, "", "")
                    self.generator.main()
                config = json.loads(Path(self.generator.CFG).read_text())
                rules = config["route"]["rules"]
                forced = {"domain": ["aquadx.hydev.org"], "action": "route", "outbound": "select"}
                self.assertIn(forced, rules)
                for index, rule in enumerate(rules):
                    if rule.get("outbound") == "direct":
                        self.assertLess(rules.index(forced), index)
                self.assertEqual(rules[0], {"action": "sniff"})
                self.assertEqual(config["dns"]["rules"][0], {
                    "domain": ["aquadx.hydev.org"], "server": "proxy-dns",
                })
                self.assertEqual(config["route"]["final"], "select")
                check.assert_called_once_with(
                    ["sing-box", "check", "-c", self.generator.CFG + ".tmp"],
                    capture_output=True, text=True,
                )

    def test_invalid_config_keeps_existing_file(self):
        path = Path(self.generator.CFG)
        path.write_text("existing config")
        with patch.object(self.generator.subprocess, "run") as check:
            check.return_value = subprocess.CompletedProcess([], 1, "", "invalid")
            with self.assertRaisesRegex(SystemExit, "CONFIG CHECK FAILED"):
                self.generator.main()
        self.assertEqual(path.read_text(), "existing config")


if __name__ == "__main__":
    unittest.main()
