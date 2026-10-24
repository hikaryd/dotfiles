"""Offline contracts. No third-party imports, live catalog or user history."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("anime_backend", Path(__file__).with_name("backend.py"))
backend = importlib.util.module_from_spec(spec)
spec.loader.exec_module(backend)


class BridgeContracts(unittest.TestCase):
    def test_catalog_view_preserves_identity_and_cyrillic(self):
        data = {"id": 9542, "name": {"main": "Фрирен"}, "description": "Магия<br>и <b>путь</b>", "year": 2023, "type": {"description": "ТВ"}, "episodes_total": 28}
        item = backend.item_view(data, "5 Фантом")
        self.assertEqual(item["title"], "Фрирен")
        self.assertEqual(item["description"], "Магия\nи путь")
        self.assertEqual(item["data"], data)
        self.assertEqual(item["last_episode"], "5 Фантом")
        self.assertEqual(item["count"], 28)

    def test_bad_requests_fail_before_loading_external_packages(self):
        for request in [[], {"op": "shell"}, {"op": "search", "query": "  "}, {"op": "episodes", "item": {"data": {"id": "../../private"}}}, {"op": "resolve", "episode": {"data": None}}]:
            with self.subTest(request=request), self.assertRaises(ValueError):
                backend.validate(request)

    def test_mpv_options_are_split_without_a_shell(self):
        self.assertEqual(backend.player_options('--title="Фрирен и путь" --vo=null'), ["--title=Фрирен и путь", "--vo=null"])
        self.assertEqual(backend.player_options('$(touch never)'), ["$(touch", "never)"])
        with self.assertRaises(ValueError):
            backend.player_options('"unterminated')

    def test_history_filters_other_providers_and_skips_invalid_rows(self):
        data = {"id": 9542, "name": {"main": "Фрирен"}}
        entries = [{"extractor_name": "anilibria", "episode": "5 Фантом", "data": {"data": {"data": data}}}, {"extractor_name": "other", "data": {}}, {"extractor_name": "anilibria", "data": {}}, {"extractor_name": "anilibria", "data": None}]
        items = backend.history_views(entries)
        self.assertEqual(len(items), 1)
        self.assertEqual(items[0]["last_episode"], "5 Фантом")
        self.assertEqual(items[0]["data"]["id"], 9542)


if __name__ == "__main__":
    unittest.main()
