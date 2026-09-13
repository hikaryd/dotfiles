"""Offline portable-source checks. No router, VPS or phone is contacted."""

import ipaddress
import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).parent


class PortabilityTests(unittest.TestCase):
    def test_pixel_scope_has_no_retired_vm_instructions(self):
        pixel = ROOT / "pixel-server"
        self.assertFalse((pixel / "DOCKER-STATUS.md").exists())
        self.assertFalse((pixel / "avf-probe").exists())
        for path in [ROOT / "PIXEL-BROWSER.md", *pixel.rglob("*.md")]:
            self.assertIsNone(
                re.search(r"docker|докер|avf-probe", path.read_text(), re.IGNORECASE),
                str(path),
            )

    def test_owner_ssh_inventory_cannot_be_added_by_git_add_all(self):
        repository = ROOT.parent
        if not (repository / ".git").exists():
            self.skipTest("source archive has no Git ignore evaluator")
        result = subprocess.run(
            ["git", "check-ignore", "--no-index", "config/ssh/pixel-owner.conf"],
            cwd=repository, capture_output=True, text=True, check=False,
        )
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_vpn_only_install_without_pixel_tree_or_tools(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for name in (
                "install.sh",
                "generate.py",
                "router-setup.py",
                "peer-register.sh",
                "vps-setup.sh",
            ):
                shutil.copyfile(ROOT / name, root / name)
            binary = root / "bin"
            binary.mkdir()
            for name in (
                "dirname",
                "mktemp",
                "rm",
                "sed",
                "head",
                "tee",
                "grep",
                "cat",
            ):
                (binary / name).symlink_to(shutil.which(name))
            scripts = {
                "ssh": "printf '===VARS===\\nPANEL_SECRET=example\\n===END===\\n'\n",
                "scp": "exit 0\n",
                "curl": "printf 200\n",
                "python3": "echo 'unexpected local router invocation' >&2; exit 99\n",
            }
            for name, script in scripts.items():
                target = binary / name
                target.write_text("#!/bin/sh\n" + script)
                target.chmod(0o700)
            result = subprocess.run(
                [
                    "/bin/bash",
                    str(root / "install.sh"),
                    "--skip-router",
                    "--vps",
                    "root@203.0.113.20",
                    "--sub-url",
                    "https://vpn.example/sub/EXAMPLE",
                ],
                env={**os.environ, "PATH": str(binary)},
                capture_output=True,
                text=True,
                timeout=10,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn("Стек развёрнут", result.stdout)
            self.assertFalse((root / "pixel-server").exists())
            for tool in ("adb", "java", "node", "termux", "chrome"):
                self.assertFalse((binary / tool).exists())

    def test_no_personal_lan_user_paths_or_device_ids(self):
        # Documentation IPv4s are TEST-NET; tunnel defaults are generic, not hosts.
        patterns = [
            r"/Users/[A-Za-z0-9_.-]+",
            r"\b192\.168\.\d+\.\d+\b",
            r"-----BEGIN (?:OPENSSH |RSA |EC )?PRIVATE KEY-----",
            r"ssh-(?:ed25519|rsa) [A-Za-z0-9+/]{40,}",
            r"SHA256:[A-Za-z0-9+/]{40,}",
            r"(?:[0-9A-Fa-f]{2}:){31}[0-9A-Fa-f]{2}",
            r'(?m)^SERIAL\s*=\s*"(?!EXAMPLE|__)',
        ]
        for path in ROOT.rglob("*"):
            if not path.is_file() or any(
                part in {".omx", ".omc", "__pycache__", "private", "rendered"}
                for part in path.parts
            ):
                continue
            text = path.read_text()
            for pattern in patterns:
                self.assertIsNone(re.search(pattern, text), str(path))
            # Public DNS services are intentional protocol defaults, not personal VPSs.
            public_dns = {"1.1.1.1", "8.8.8.8", "9.9.9.9", "77.88.8.8"}
            for match in re.finditer(
                r"(?<![\w.])(?:\d{1,3}\.){3}\d{1,3}(?![\w.])", text
            ):
                try:
                    address = ipaddress.IPv4Address(match.group())
                except ValueError:
                    continue
                if address.is_global:
                    self.assertIn(str(address), public_dns, str(path))
            example_macs = {"02:00:00:00:00:01", "00:11:22:33:44:55"}
            for match in re.finditer(
                r"(?i)(?<![0-9a-f:])(?:[0-9a-f]{2}:){5}[0-9a-f]{2}(?![0-9a-f:])", text
            ):
                self.assertIn(match.group().lower(), example_macs, str(path))


if __name__ == "__main__":
    unittest.main()
