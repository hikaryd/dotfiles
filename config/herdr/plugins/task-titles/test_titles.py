"""Exercise ownership and event feedback with Herdr-shaped snapshots."""

import copy
import json
import subprocess
import unittest
from unittest.mock import patch

import titles


def snapshot():
    return {
        "panes": [{"pane_id": "w1:p1", "terminal_id": "terminal-one", "tab_id": "w1:t1", "agent": "codex", "terminal_title_stripped": "Исправить поиск статуса | podbor"}],
        "tabs": [{"tab_id": "w1:t1", "label": "nu"}],
        "layouts": [{"tab_id": "w1:t1", "focused_pane_id": "w1:p1"}],
    }


class TitlesTest(unittest.TestCase):
    def setUp(self):
        self.snapshot = snapshot()
        self.state = {"panes": {}, "tabs": {}}
        self.calls = []

    def command(self, *args):
        self.calls.append(args)
        if args[:2] == ("tab", "rename"):
            next(t for t in self.snapshot["tabs"] if t["tab_id"] == args[2])["label"] = args[3]
        else:
            pane = next(p for p in self.snapshot["panes"] if p["pane_id"] == args[2])
            if "--clear-title" in args:
                pane.pop("title", None)
            else:
                pane["title"] = args[args.index("--title") + 1]

    def sync(self):
        titles.synchronize(copy.deepcopy(self.snapshot), self.state, self.command)

    def test_native_title_maps_to_pane_and_tab_without_feedback_loop(self):
        self.sync()
        self.assertEqual(self.snapshot["panes"][0]["title"], "Исправить поиск статуса")
        self.assertEqual(self.snapshot["tabs"][0]["label"], "Исправить поиск статуса")
        self.calls.clear()
        self.sync()  # tab.renamed and later agent/focus events are no-ops.
        self.assertEqual(self.calls, [])

    def test_title_changes_follow_focus_and_append_count(self):
        self.snapshot["panes"].append({"pane_id": "w1:p2", "tab_id": "w1:t1", "agent": "codex", "terminal_title_stripped": "Проверить API | podbor"})
        self.sync()
        self.assertEqual(self.snapshot["tabs"][0]["label"], "Исправить поиск статуса · 2")
        self.snapshot["layouts"][0]["focused_pane_id"] = "w1:p2"
        self.sync()
        self.assertEqual(self.snapshot["tabs"][0]["label"], "Проверить API · 2")

    def test_manual_tab_rename_is_preserved_even_to_shell_name(self):
        self.sync()
        self.snapshot["tabs"][0]["label"] = "zsh"
        self.sync()
        self.snapshot["panes"][0]["terminal_title_stripped"] = "Другая задача | podbor"
        self.sync()
        self.assertEqual(self.snapshot["tabs"][0]["label"], "zsh")
        self.assertTrue(self.state["tabs"]["w1:t1"]["manual"])

    def test_existing_manual_names_and_foreign_metadata_are_preserved(self):
        self.snapshot["tabs"][0]["label"] = "Важная работа"
        self.snapshot["panes"][0]["label"] = "Моя панель"
        self.sync()
        self.assertEqual(self.calls, [])
        self.snapshot["panes"][0].pop("label")
        self.snapshot["panes"][0]["title"] = "Other integration"
        self.sync()
        self.assertEqual(self.calls, [])

    def test_exit_clears_only_our_metadata_and_restores_original_tab(self):
        self.sync()
        self.calls.clear()
        self.snapshot["panes"][0].pop("agent")
        self.sync()
        self.assertNotIn("title", self.snapshot["panes"][0])
        self.assertEqual(self.snapshot["tabs"][0]["label"], "nu")
        self.assertIn("--clear-title", self.calls[0])
        self.assertIn(titles.SOURCE, self.calls[0])

    def test_manual_pane_label_releases_our_previous_title(self):
        self.sync()
        self.snapshot["panes"][0]["label"] = "Pinned"
        self.sync()
        self.assertNotIn("title", self.snapshot["panes"][0])
        self.assertEqual(self.state["panes"], {})

    def test_sanitize_suffix_controls_and_word_boundary(self):
        self.assertEqual(titles.task_title("\x1b[31mПочинить\x1b[0m   поиск\u202e | podbor"), "Починить поиск")
        self.assertEqual(titles.task_title("hello long world | repo", 13), "hello long…")
        self.assertLessEqual(len(titles.task_title("Очень длинное название задачи которое нужно обрезать | repo", 32)), 32)

    def test_failed_rename_does_not_claim_a_successful_update(self):
        def fail(*args):
            if args[:2] == ("tab", "rename"):
                raise RuntimeError("test API unavailable")
            self.command(*args)
        with self.assertRaisesRegex(RuntimeError, "API unavailable"):
            titles.synchronize(copy.deepcopy(self.snapshot), self.state, fail)
        self.assertEqual(self.state["tabs"]["w1:t1"]["last"], "nu")
        self.sync()
        self.assertEqual(self.snapshot["tabs"][0]["label"], "Исправить поиск статуса")

    def test_semantic_name_survives_native_titles_and_updates_both_surfaces(self):
        self.sync()
        titles.set_title(self.snapshot, self.state, "w1:p1", "Статусы заявок · поиск закрытых", "session-one")
        self.sync()
        self.snapshot["panes"][0]["terminal_title_stripped"] = "Исследовать баг | podbor"
        self.sync()
        self.assertEqual(self.snapshot["panes"][0]["title"], "Статусы заявок · поиск закрытых")
        self.assertEqual(self.snapshot["tabs"][0]["label"], "Статусы заявок · поиск закрытых")
        self.calls.clear()
        self.sync()
        self.assertEqual(self.calls, [])

    def test_semantic_name_expires_on_session_or_terminal_replacement(self):
        titles.set_title(self.snapshot, self.state, "w1:p1", "Точная задача", "session-one")
        self.sync()
        self.snapshot["panes"][0]["agent_session"] = {"agent": "codex", "value": "session-two"}
        self.sync()
        self.assertEqual(self.state["semantic"], {})
        self.assertEqual(self.snapshot["panes"][0]["title"], "Исправить поиск статуса")
        titles.set_title(self.snapshot, self.state, "w1:p1", "Новая задача", "session-two")
        self.sync()
        self.snapshot["panes"][0]["terminal_id"] = "replacement"
        self.sync()
        self.assertEqual(self.state["semantic"], {})

    def test_semantic_name_binds_late_identity_and_clears_after_exit(self):
        titles.set_title(self.snapshot, self.state, "w1:p1", "Точная задача")
        self.snapshot["panes"][0]["agent_session"] = {"agent": "codex", "value": "session-one"}
        self.sync()
        self.assertEqual(self.state["semantic"]["w1:p1"]["session_id"], "session-one")
        self.snapshot["panes"][0].pop("agent")
        self.sync()
        self.assertEqual(self.state["semantic"], {})
        self.assertNotIn("title", self.snapshot["panes"][0])

    def test_semantic_set_rejects_wrong_target_and_preserves_manual_labels(self):
        self.snapshot["panes"][0]["agent_session"] = {"agent": "codex", "value": "session-one"}
        with self.assertRaises(ValueError):
            titles.set_title(self.snapshot, self.state, "w1:p1", "Title", "other-session")
        with self.assertRaises(ValueError):
            titles.set_title(self.snapshot, self.state, "missing", "Title")
        with self.assertRaises(ValueError):
            titles.set_title(self.snapshot, self.state, "w1:p1", " \u202e ")
        self.snapshot["panes"][0]["label"] = "Pinned pane"
        self.snapshot["tabs"][0]["label"] = "Pinned tab"
        titles.set_title(self.snapshot, self.state, "w1:p1", "Точная задача", "session-one")
        self.sync()
        self.assertEqual(self.calls, [])

    def test_real_cli_empty_metadata_acknowledgement_keeps_ownership(self):
        def cli(argv, **kwargs):
            args = tuple(argv[1:])
            self.command(*args)
            output = "" if args[:2] == ("pane", "report-metadata") else json.dumps({"result": {}})
            return subprocess.CompletedProcess(argv, 0, output, "")
        with patch.object(titles.subprocess, "run", side_effect=cli):
            titles.synchronize(copy.deepcopy(self.snapshot), self.state, titles.run_herdr)
            self.assertEqual(self.state["panes"]["w1:p1"], "Исправить поиск статуса")
            titles.set_title(self.snapshot, self.state, "w1:p1", "3742879 · Создание вакансии")
            titles.synchronize(copy.deepcopy(self.snapshot), self.state, titles.run_herdr)
        self.assertEqual(self.snapshot["panes"][0]["title"], "3742879 · Создание вакансии")
        self.assertEqual(self.state["panes"]["w1:p1"], "3742879 · Создание вакансии")

    def test_only_metadata_accepts_empty_success_output(self):
        with patch.object(titles.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, "", "")):
            with self.assertRaises(json.JSONDecodeError):
                titles.run_herdr("api", "snapshot")
        with patch.object(titles.subprocess, "run", return_value=subprocess.CompletedProcess([], 1, "", "failed")):
            with self.assertRaisesRegex(RuntimeError, "exit 1"):
                titles.run_herdr("pane", "report-metadata", "w1:p1", "--source", titles.SOURCE, "--title", "Title")


if __name__ == "__main__":
    unittest.main()
