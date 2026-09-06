"""Результат установки сохраняется до вывода в оборванное SSH-соединение."""

import os
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[1] / "vpn/vps-setup.sh"


class VpsResultTests(unittest.TestCase):
    def test_result_survives_broken_stdout(self):
        footer = SCRIPT.read_text().split("# ключи роутеров тут не печатаем", 1)[1]
        footer = footer.split("\n", 1)[1]
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            result_file = root / "result"
            (root / "server.pub").write_text("test-public-key")
            wrapper = root / "footer.sh"
            wrapper.write_text(
                'set -euo pipefail\n'
                'cat() { if [[ "$1" == /etc/sing-box/secret.txt ]]; then '
                'printf test-secret; else /bin/cat "$@"; fi; }\n' + footer,
            )
            env = os.environ | dict.fromkeys(["Jc", "Jmin", "Jmax", "S1", "S2", "H1", "H2", "H3", "H4"], "1")
            env["INSTALL_RESULT_FILE"] = str(result_file)
            reader, writer = os.pipe()
            os.close(reader)
            try:
                result = subprocess.run(
                    ["bash", str(wrapper)], cwd=root, env=env, stdout=writer,
                    stderr=subprocess.PIPE, check=False, timeout=5,
                )
            finally:
                os.close(writer)
            self.assertNotEqual(result.returncode, 0)
            saved = result_file.read_text()
            self.assertTrue(saved.startswith("===VARS===\n"))
            self.assertTrue(saved.endswith("===END===\n"))
            self.assertIn("PANEL_SECRET=test-secret", saved)
            self.assertEqual(stat.S_IMODE(result_file.stat().st_mode), 0o600)
            self.assertFalse(Path(str(result_file) + ".tmp").exists())


if __name__ == "__main__":
    unittest.main()
