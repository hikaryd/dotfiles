"""Exercise uninstall's read-only link inventory, never its mutating shell flow."""

import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
START = 'LINK_TARGETS=$(python3 - "$BASEDIR" <<\'PY\'\n'
END = "\nPY\n)"


class UninstallLinksTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.base = Path(temporary.name)
        (self.base / "steps").mkdir()
        (self.base / "steps/terminal.yml").write_text(
            "- link:\n"
            "    ~/.config/example:\n"
            "      path: config/example\n"
            "    ~/Library/Application Support/example/config.json:\n"
            "      path: config/example.json\n"
            "- shell:\n"
            "    - command: echo not-a-link\n"
        )
        self.expected = [
            "~/.config/example",
            "~/Library/Application Support/example/config.json",
        ]

    def inventory(self):
        # -S excludes system/user packages: a global PyYAML install must not be
        # necessary. Execute just the actual inventory code, not a reimplementation.
        source = (ROOT / "uninstall").read_text().split(START, 1)[1].split(END, 1)[0]
        result = subprocess.run(
            [sys.executable, "-S", "-c", source, str(self.base)],
            env={**os.environ, "PYTHONPATH": "", "PYTHONDONTWRITEBYTECODE": "1"},
            capture_output=True,
            text=True,
            check=True,
        )
        return result.stdout.splitlines()

    def test_uses_dotbot_yaml_without_global_packages(self):
        library = self.base / "dotbot/lib/pyyaml/lib"
        library.parent.mkdir(parents=True)
        library.symlink_to(ROOT / "dotbot/lib/pyyaml/lib", target_is_directory=True)
        # Quoted YAML keys require the real parser, not the minimal fallback.
        (self.base / "steps/quoted.yml").write_text(
            '- link:\n    "~/quoted config":\n      path: config/example\n'
        )
        self.assertEqual(self.inventory(), ["~/quoted config", *self.expected])

    def test_fallback_without_dotbot_preserves_unquoted_paths_with_spaces(self):
        self.assertEqual(self.inventory(), self.expected)


if __name__ == "__main__":
    unittest.main()
