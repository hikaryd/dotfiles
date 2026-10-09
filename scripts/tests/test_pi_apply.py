import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("pi_apply", ROOT / "scripts/pi-apply.py")
pi_apply = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pi_apply)


class PiApplyTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.agent = Path(self.temporary.name) / "agent"
        self.agent.mkdir()

    def write(self, name, data):
        path = self.agent / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(data))
        return path

    def read(self, name):
        return json.loads((self.agent / name).read_text())

    def test_preserves_settings_models_auth_and_unrelated_resources(self):
        self.write("settings.json", {
            "theme": "dark", "httpProxy": "http://127.0.0.1:10810",
            "defaultModel": "gpt-6.1-sol",
            "modelThinkingLevels": {"openai/gpt-6.1-sol": "xhigh", "local/other": "low"},
            "terminal": {"showImages": False}, "extensions": ["other.ts"],
            "deviceId": "local-device", "lastChangelogVersion": "local-version",
        })
        self.write("models.json", {"providers": {"openai": {"headers": {"X-Test": "keep"}, "modelOverrides": {"gpt-6.1-sol": {"contextWindow": 272000, "maxTokens": 16384}, "other-model": {"contextWindow": 123456}}}, "local": {"baseUrl": "http://localhost:1234/v1"}}})
        auth = self.write("auth.json", {"openai": {"type": "oauth", "access": "fixture-not-a-token"}})
        original_auth = auth.read_bytes()
        other = self.agent / "extensions/other.ts"
        other.parent.mkdir()
        other.write_text("keep me")
        pi_apply.apply(self.agent)
        settings = self.read("settings.json")
        self.assertEqual(settings["theme"], "ayu")
        self.assertEqual(settings["defaultModel"], "gpt-6.1-sol")
        self.assertEqual(settings["httpProxy"], "http://127.0.0.1:10810")
        self.assertEqual(settings["modelThinkingLevels"]["openai/gpt-6.1-sol"], "xhigh")
        self.assertEqual(settings["modelThinkingLevels"]["local/other"], "low")
        self.assertEqual(settings["terminal"], {"showImages": False, "showTerminalProgress": True})
        self.assertEqual(settings["extensions"], ["other.ts"])
        self.assertEqual(settings["deviceId"], "local-device")
        self.assertEqual(settings["lastChangelogVersion"], "local-version")
        providers = self.read("models.json")["providers"]
        self.assertEqual(providers["openai"]["modelOverrides"]["gpt-6.1-sol"], {"contextWindow": 400000, "maxTokens": 16384})
        self.assertEqual(providers["openai"]["headers"], {"X-Test": "keep"})
        self.assertEqual(providers["openai"]["modelOverrides"]["other-model"]["contextWindow"], 123456)
        self.assertEqual(providers["local"]["baseUrl"], "http://localhost:1234/v1")
        self.assertEqual(auth.read_bytes(), original_auth)
        self.assertEqual(other.read_text(), "keep me")
        self.assertTrue((self.agent / "themes/ayu.json").is_symlink())
        self.assertTrue((self.agent / "extensions/dots-ayu-bar").is_symlink())
        backup = next(self.agent.glob("settings.json.bak-*"))
        self.assertEqual(json.loads(backup.read_text())["theme"], "dark")

    def test_idempotent_and_new_files_are_private(self):
        pi_apply.apply(self.agent)
        files_before = sorted(str(path.relative_to(self.agent)) for path in self.agent.rglob("*"))
        settings_before = (self.agent / "settings.json").read_bytes()
        models_before = (self.agent / "models.json").read_bytes()
        pi_apply.apply(self.agent)
        self.assertEqual(sorted(str(path.relative_to(self.agent)) for path in self.agent.rglob("*")), files_before)
        self.assertEqual((self.agent / "settings.json").read_bytes(), settings_before)
        self.assertEqual((self.agent / "models.json").read_bytes(), models_before)
        self.assertEqual((self.agent / "models.json").stat().st_mode & 0o777, 0o600)
        self.assertEqual((self.agent / "settings.json").stat().st_mode & 0o777, 0o600)

    def test_applies_portable_personal_preferences(self):
        self.write("settings.json", {
            "defaultProvider": "local", "defaultModel": "other",
            "modelThinkingLevels": {"openai/gpt-6.1-sol": "low"},
            "hideThinkingBlock": False, "enableInstallTelemetry": True,
            "quietStartup": True,
        })
        pi_apply.apply(self.agent)
        expected = pi_apply.read_json(ROOT / "config/pi/settings.json")
        self.assertEqual(self.read("settings.json"), {**expected, "quietStartup": True})
        self.assertNotIn("deviceId", expected)
        self.assertNotIn("lastChangelogVersion", expected)

    def test_conflicting_resource_is_backed_up(self):
        old = self.agent / "themes/ayu.json"
        old.parent.mkdir()
        old.write_text("old theme")
        pi_apply.apply(self.agent)
        self.assertEqual(next((self.agent / "backups").glob("ayu.json.bak-*")).read_text(), "old theme")
        self.assertTrue(old.is_symlink())

    def test_archived_extension_is_not_left_in_auto_discovery(self):
        old = self.agent / "extensions/dots-ayu-bar"
        old.mkdir(parents=True)
        (old / "index.ts").write_text("old extension")
        pi_apply.apply(self.agent)
        backup = next((self.agent / "backups").glob("dots-ayu-bar.bak-*"))
        self.assertEqual((backup / "index.ts").read_text(), "old extension")
        self.assertEqual(list(old.parent.iterdir()), [old])

    def test_existing_settings_symlink_is_preserved(self):
        target = Path(self.temporary.name) / "personal-settings.json"
        target.write_text('{"theme":"dark","quietStartup":true}')
        settings = self.agent / "settings.json"
        settings.symlink_to(target)
        pi_apply.apply(self.agent)
        self.assertTrue(settings.is_symlink())
        expected = pi_apply.read_json(ROOT / "config/pi/settings.json")
        self.assertEqual(json.loads(target.read_text()), {**expected, "quietStartup": True})
        self.assertTrue(list(target.parent.glob("personal-settings.json.bak-*")))

    def test_malformed_config_changes_nothing(self):
        for name in ["settings.json", "models.json"]:
            with self.subTest(name=name):
                path = self.agent / name
                path.write_text("bad JSON")
                with self.assertRaises(ValueError):
                    pi_apply.apply(self.agent)
                self.assertEqual(path.read_text(), "bad JSON")
                self.assertFalse((self.agent / "themes").exists())
                self.assertFalse(list(self.agent.glob("*.bak-*")))
                path.unlink()


if __name__ == "__main__":
    unittest.main()
