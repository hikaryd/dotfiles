"""Exercise the actual dependency command without changing the host."""

import os
import subprocess
import tempfile
import textwrap
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


class SnapzyInstallTests(unittest.TestCase):
    def run_step(self, *, installed=True, app=False, declared=True, **statuses):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / "Brewfile").write_text(
                'cask "duongductrong/snapzy/snapzy"\n' if declared else ""
            )
            app_path = root / "Snapzy.app"
            if app:
                app_path.mkdir()
            brew = root / "brew"
            brew.write_text(
                "#!/bin/bash\n"
                'echo "$*" >> "$CALLS"\n'
                'case "$1" in\n'
                '  list) exit "$LIST_STATUS" ;;\n'
                '  uninstall) exit "${UNINSTALL_STATUS:-0}" ;;\n'
                '  bundle) exit "${BUNDLE_STATUS:-0}" ;;\n'
                "  trust) exit 0 ;;\n"
                "  *) exit 99 ;;\n"
                "esac\n"
            )
            brew.chmod(0o755)
            source = (ROOT / "steps/dependencies.yml").read_text()
            command = textwrap.dedent(
                source.split("    - command: |\n", 1)[1].split("      description:", 1)[
                    0
                ]
            ).replace("/Applications/Snapzy.app", str(app_path))
            calls = root / "calls"
            result = subprocess.run(
                ["bash", "-c", command],
                cwd=root,
                env={
                    **os.environ,
                    "PATH": f"{root}:/usr/bin:/bin",
                    "CALLS": str(calls),
                    "LIST_STATUS": "0" if installed else "1",
                    **statuses,
                },
                capture_output=True,
                text=True,
                check=False,
            )
            return result, calls.read_text().splitlines()

    def test_missing_installed_app_is_reconciled_before_bundle(self):
        result, calls = self.run_step()
        self.assertEqual(result.returncode, 0, result.stderr)
        uninstall = "uninstall --cask duongductrong/snapzy/snapzy --force"
        self.assertIn(uninstall, calls)
        self.assertLess(calls.index(uninstall), calls.index("bundle --file Brewfile"))
        self.assertNotIn("--zap", " ".join(calls))

    def test_existing_app_is_not_removed(self):
        result, calls = self.run_step(app=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any(call.startswith("uninstall") for call in calls))

    def test_fresh_install_does_not_uninstall(self):
        result, calls = self.run_step(installed=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any(call.startswith("uninstall") for call in calls))

    def test_undeclared_snapzy_is_untouched(self):
        result, calls = self.run_step(declared=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any("snapzy" in call for call in calls))

    def test_cleanup_failure_stops_before_bundle(self):
        result, calls = self.run_step(UNINSTALL_STATUS="7")
        self.assertEqual(result.returncode, 7, result.stderr)
        self.assertNotIn("bundle --file Brewfile", calls)

    def test_bundle_failure_is_propagated(self):
        result, _ = self.run_step(app=True, BUNDLE_STATUS="8")
        self.assertEqual(result.returncode, 8)


if __name__ == "__main__":
    unittest.main()
