import os
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


class PiInstallTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.home = Path(self.temporary.name)
        self.bin = self.home / "bin"
        self.bin.mkdir()
        # Isolate PATH from the host's installed Pi, node and npm.
        for name in ("mkdir", "ln"):
            (self.bin / name).symlink_to(Path("/bin") / name)
        self.env = {**os.environ, "HOME": str(self.home), "PATH": str(self.bin)}

    def executable(self, path, content):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("#!/bin/sh\nset -eu\n" + content + "\n")
        path.chmod(0o755)
        return path

    def run_install(self):
        return subprocess.run(
            ["/bin/sh", str(ROOT / "scripts/pi-install.sh")],
            env=self.env, text=True, capture_output=True,
        )

    def test_existing_cli_is_reused_without_npm(self):
        self.executable(self.bin / "pi", 'printf "existing-pi\\n"')
        result = self.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("existing-pi", result.stdout)
        self.assertFalse((self.home / ".local").exists())

    def test_managed_launcher_outside_path_is_linked(self):
        launcher = self.executable(self.home / ".pi/agent/bin/pi", 'printf "managed-pi\\n"')
        result = self.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.home / ".local/bin/pi").resolve(), launcher.resolve())
        self.assertIn("managed-pi", result.stdout)

    def test_fresh_install_is_pinned_user_owned_and_ignores_scripts(self):
        self.executable(self.bin / "node", "exit 0")
        self.executable(self.bin / "npm", '''printf '%s\\n' "$@" > "$HOME/npm-args"
mkdir -p "$HOME/.local/bin"
printf '#!/bin/sh\\nprintf "new-pi\\\\n"\\n' > "$HOME/.local/bin/pi"
/bin/chmod 755 "$HOME/.local/bin/pi"''')
        result = self.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)
        args = (self.home / "npm-args").read_text().splitlines()
        self.assertEqual(args, [
            "install", "-g", "--prefix", str(self.home / ".local"),
            "--ignore-scripts", "--no-fund", "--no-audit",
            "@earendil-works/pi-coding-agent@1.1.0",
        ])
        self.assertIn("new-pi", result.stdout)
        self.env["PATH"] = str(self.home / ".local/bin") + ":" + str(self.bin)
        (self.home / "npm-args").unlink()
        self.assertEqual(self.run_install().returncode, 0)
        self.assertFalse((self.home / "npm-args").exists())

    def test_conflicting_launcher_is_preserved(self):
        launcher = self.home / ".local/bin/pi"
        launcher.parent.mkdir(parents=True)
        launcher.symlink_to(self.home / "missing")
        result = self.run_install()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("preserved", result.stderr)
        self.assertTrue(launcher.is_symlink())
        self.assertEqual(launcher.readlink(), self.home / "missing")

    def test_old_or_missing_node_does_not_install(self):
        self.executable(self.bin / "npm", 'printf "unexpected-npm\\n"')
        result = self.run_install()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Node.js", result.stderr)
        self.executable(self.bin / "node", "exit 1")
        result = self.run_install()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("22.19", result.stderr)
        self.assertNotIn("unexpected-npm", result.stdout)

    def test_npm_failure_is_not_hidden(self):
        self.executable(self.bin / "node", "exit 0")
        self.executable(self.bin / "npm", "exit 42")
        self.assertEqual(self.run_install().returncode, 42)
        self.assertFalse((self.home / ".local/bin/pi").exists())


if __name__ == "__main__":
    unittest.main()
