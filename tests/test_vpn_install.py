"""Проверка транспорта установщика: SSH только к VPS, роутер через HTTP."""

import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


class InstallTests(unittest.TestCase):
    def test_router_setup_never_uses_ssh(self):
        for scenario in ("skip", "normal", "disconnect", "failed", "incomplete", "truncated"):
            with self.subTest(scenario=scenario), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                log = root / "calls"
                for name in ("install.sh", "generate.py", "vps-setup.sh", "peer-register.sh"):
                    shutil.copy(ROOT / "vpn" / name, root / name)
                (root / "router-setup.py").write_text(
                    "import os\n"
                    "from pathlib import Path\n"
                    "with Path(os.environ['CALL_LOG']).open('a') as f:\n"
                    "    f.write('router-http ' + os.environ.get('ACTION', 'install') + '\\n')\n"
                    "print('NAME=test-router\\nCURPUB=test-public-key')\n"
                )
                binaries = root / "bin"
                binaries.mkdir()
                scripts = {
                    "ssh": '''#!/bin/bash
echo "ssh $*" >> "$CALL_LOG"
[[ "$*" != *"192.168.254.1"* ]] || exit 99
if [[ "$*" == *"bash /tmp/awg-vps-setup.sh"* ]]; then
  case "$SCENARIO" in
    disconnect|incomplete|truncated) exit 255;;
    failed) exit 1;;
  esac
fi
if [[ "$*" == *"systemctl is-active"* ]]; then
  [[ "$SCENARIO" != incomplete ]] || exit 1
  [[ "$*" == *".install-result-"* ]] || exit 99
  if [[ "$SCENARIO" == truncated ]]; then echo '===VARS==='; exit 0; fi
fi
cat <<'VARS'
===VARS===
SERVER_PUB=test-server-public-key
ROUTER_KEY=test-router-private-key
AWG_PARAMS=3 10 30 20 30 100 200 300 400
ROUTER_IP=10.8.2.2
TUN_SRV=10.8.2.1
TUN_CIDR=10.8.2.0/24
AWG_PORT=51820
MTU=1280
PANEL_SECRET=test-secret
===END===
VARS
''',
                    "scp": '#!/bin/bash\necho "scp $*" >> "$CALL_LOG"\n',
                    "sshpass": '#!/bin/bash\necho "sshpass $*" >> "$CALL_LOG"\nexit 99\n',
                    "expect": '#!/bin/bash\necho "expect $*" >> "$CALL_LOG"\nexit 99\n',
                    "curl": '#!/bin/bash\necho "curl $*" >> "$CALL_LOG"\nprintf 200\n',
                    "sleep": '#!/bin/bash\nexit 0\n',
                }
                for name, script in scripts.items():
                    path = binaries / name
                    path.write_text(script)
                    path.chmod(0o755)
                args = ["bash", str(root / "install.sh"), "--vps", "root@192.0.2.10",
                        "--router-pass", "test-password"]
                args += ["--skip-vps"] if scenario == "skip" else ["--sub-url", "https://example.invalid/sub"]
                result = subprocess.run(
                    args, input="", capture_output=True, text=True, errors="replace", timeout=15, check=False,
                    env=os.environ | {"PATH": f"{binaries}:{os.environ['PATH']}", "CALL_LOG": str(log),
                                      "SCENARIO": scenario},
                )
                calls = log.read_text()
                if scenario in ("failed", "incomplete", "truncated"):
                    self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertNotIn("router-http install", calls)
                    continue
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertIn("root@192.0.2.10", calls)
                self.assertIn("router-http identify", calls)
                self.assertIn("router-http install", calls, result.stdout + result.stderr)
                self.assertIn("curl --noproxy *", calls)
                self.assertNotIn("sshpass", calls)
                self.assertNotIn("expect", calls)
                self.assertNotIn("root@192.168.254.1", calls)
                self.assertNotIn("kill-switch", result.stdout)
                if scenario == "disconnect":
                    self.assertIn("systemctl is-active", calls)

    def test_password_prompt_disables_terminal_echo(self):
        source = (ROOT / "vpn/install.sh").read_text()
        self.assertIn('ROUTER_PASS=$(ask_secret ', source)
        self.assertIn('read -rsp ', source)

    def test_help_has_no_router_ssh_options(self):
        result = subprocess.run(
            ["bash", str(ROOT / "vpn/install.sh"), "--help"],
            capture_output=True, text=True, check=True,
        )
        self.assertIn("SSH к VPS", result.stdout)
        self.assertNotIn("--router-ssh-port", result.stdout)
        self.assertNotIn("--no-lan-hook", result.stdout)


if __name__ == "__main__":
    unittest.main()
