import json
import re
import unittest
from pathlib import Path


class KeyboardConfigTests(unittest.TestCase):
    def test_screen_sharing_switches_only_supported_local_layouts(self):
        config = json.loads(Path(__file__).with_name("karabiner.json").read_text())
        profile = next(p for p in config["profiles"] if p.get("selected"))
        self.assertIn({"from": {"key_code": "caps_lock"}, "to": [{"key_code": "f13"}]},
                      profile["simple_modifications"])
        rules = profile["complex_modifications"]["rules"]
        rule = next(r for r in rules if r["description"] ==
                    "Caps Lock switches local RU/EN in Screen Sharing")
        self.assertEqual(len(rule["manipulators"]), 2)
        for manipulator, source, target in zip(rule["manipulators"],
                                               ("Russian", "ABC"), ("ABC", "Russian")):
            self.assertEqual(manipulator["type"], "basic")
            self.assertEqual(manipulator["from"], {
                "key_code": "f13", "modifiers": {"optional": ["fn", "caps_lock"]}})
            self.assertEqual(manipulator["to"], [{"select_input_source": {
                "input_source_id": r"^com\.apple\.keylayout\." + target + "$"}}])
            conditions = manipulator["conditions"]
            self.assertEqual(len(conditions), 2)
            app, layout = conditions
            self.assertEqual(app["type"], "frontmost_application_if")
            self.assertEqual(app["bundle_identifiers"], [r"^com\.apple\.ScreenSharing$"])
            for other in ("com.apple.Terminal", "com.apple.ScreenSharing.extra"):
                self.assertFalse(re.search(app["bundle_identifiers"][0], other))
            self.assertEqual(layout, {"type": "input_source_if", "input_sources": [
                {"input_source_id": r"^com\.apple\.keylayout\." + source + "$"}]})


if __name__ == "__main__":
    unittest.main()
