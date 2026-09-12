import contextlib
import io
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import tab_cleaner as cleaner


class CleanerTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.directory = Path(self.temporary.name)
        self.path = self.directory / "state.json"
        self.calls = []
        self.targets = [
            {"id": "A123", "type": "page", "url": "https://private.example"}
        ]
        self.failure = False

    def fetch(self, path):
        self.calls.append(path)
        if path == "/json/list":
            return json.dumps(self.targets).encode()
        if self.failure:
            raise OSError("secret must not be logged")
        return b"Target is closing"

    def tick(self, mono, boot="boot-a"):
        with contextlib.redirect_stdout(io.StringIO()) as captured:
            result = cleaner.tick(self.directory, boot, mono, self.fetch)
        self.assertNotIn("private", captured.getvalue())
        return result

    def test_first_observation_baseline_and_no_content(self):
        self.tick(100000)
        self.assertEqual(self.calls, ["/json/list"])
        self.assertEqual(cleaner.load_state(self.path)["targets"], {"A123": 0})
        self.assertNotIn("url", self.path.read_text())
        self.assertEqual(self.path.stat().st_mode & 0o777, 0o600)

    def test_exact_boundary_navigation_does_not_reset_age(self):
        self.tick(100)
        self.targets[0]["url"] = "https://different.example"
        self.tick(100 + cleaner.TTL_SECONDS - 1)
        self.assertNotIn("/json/close/A123", self.calls)
        self.tick(100 + cleaner.TTL_SECONDS)
        self.assertEqual(self.calls[-1], "/json/close/A123")
        self.assertEqual(cleaner.load_state(self.path)["targets"], {})

    def test_new_missing_and_nonpage_targets(self):
        self.tick(0)
        self.targets = [
            {"id": "B", "type": "page"},
            {"id": "C", "type": "service_worker"},
        ]
        self.tick(100000)
        self.assertEqual(cleaner.load_state(self.path)["targets"], {"B": 0})
        self.targets.append({"id": "A123", "type": "page"})
        self.tick(100100)
        self.assertEqual(
            cleaner.load_state(self.path)["targets"], {"B": 100, "A123": 0}
        )

    def test_restart_retains_age_and_reboot_ignores_downtime(self):
        self.tick(100)
        self.tick(500)
        self.tick(900000, "boot-b")
        self.assertEqual(cleaner.load_state(self.path)["targets"]["A123"], 400)
        self.tick(900100, "boot-b")
        self.assertEqual(cleaner.load_state(self.path)["targets"]["A123"], 500)

    def test_monotonic_rollback_and_wall_clock_independence(self):
        self.tick(500)
        self.tick(700)
        with patch("time.time", side_effect=AssertionError("wall clock used")):
            self.tick(600)
            self.tick(650)
        self.assertEqual(cleaner.load_state(self.path)["targets"]["A123"], 250)

    def test_corrupt_state_resets_without_close(self):
        for content in ("broken", "[]", '{"version":1}', '{"targets":{"A123":NaN}}'):
            with self.subTest(content=content):
                self.path.write_text(content)
                self.calls.clear()
                self.tick(100000)
                self.assertEqual(self.calls, ["/json/list"])
                self.assertEqual(cleaner.load_state(self.path)["targets"]["A123"], 0)

    def test_malformed_snapshot_aborts_without_state_change_or_close(self):
        self.tick(0)
        before = self.path.read_bytes()
        invalid = [
            None,
            {},
            [None],
            [{"id": "../close/x", "type": "page"}],
            [{"id": "A", "type": "page"}, {"id": "A", "type": "page"}],
            [{"id": "A"}],
            [{"id": "A/evil", "type": "page"}],
            [{"id": "A" * 129, "type": "page"}],
        ]
        for targets in invalid:
            with self.subTest(targets=targets):
                self.targets = targets
                self.calls.clear()
                with self.assertRaises(ValueError):
                    self.tick(100000)
                self.assertEqual(before, self.path.read_bytes())
                self.assertEqual(self.calls, ["/json/list"])

    def test_listing_failure_preserves_state(self):
        self.tick(0)
        before = self.path.read_bytes()
        with self.assertRaises(OSError):
            cleaner.tick(
                self.directory,
                "boot-a",
                100000,
                lambda path: (_ for _ in ()).throw(OSError("unavailable")),
            )
        self.assertEqual(self.path.read_bytes(), before)

    def test_failed_close_retried_next_tick(self):
        self.tick(0)
        self.failure = True
        self.assertFalse(self.tick(cleaner.TTL_SECONDS))
        self.assertEqual(
            cleaner.load_state(self.path)["targets"]["A123"], cleaner.TTL_SECONDS
        )
        self.failure = False
        self.assertTrue(self.tick(cleaner.TTL_SECONDS + 300))
        self.assertEqual(self.calls.count("/json/close/A123"), 2)

    def test_bad_close_body_retains_age(self):
        self.tick(0)
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertFalse(
                cleaner.tick(
                    self.directory,
                    "boot-a",
                    50000,
                    lambda path: self.fetch(path) if path == "/json/list" else b"false",
                )
            )
        self.assertIn("A123", cleaner.load_state(self.path)["targets"])

    def test_invalid_numeric_state_rebaselines(self):
        for age in (-1, True, float("inf"), cleaner.TTL_SECONDS + 1, 10**400):
            with self.subTest(age=age):
                self.path.write_text(
                    json.dumps(
                        {
                            "version": 1,
                            "boot": "boot-a",
                            "mono": 0,
                            "targets": {"A123": age},
                        }
                    )
                )
                self.calls.clear()
                self.tick(100000)
                self.assertEqual(self.calls, ["/json/list"])
                self.assertEqual(cleaner.load_state(self.path)["targets"], {"A123": 0})

    def test_transport_rejects_redirect_and_oversize(self):
        for status, data in ((302, b""), (200, b"x" * (cleaner.MAX_BYTES + 1))):
            with patch("http.client.HTTPConnection") as connection:
                response = connection.return_value.getresponse.return_value
                response.status = status
                response.read.return_value = data
                with self.assertRaises(ValueError):
                    cleaner.request("/json/list")
                connection.assert_called_once_with("127.0.0.1", 19222, timeout=5)
                connection.return_value.close.assert_called_once()


if __name__ == "__main__":
    unittest.main()
