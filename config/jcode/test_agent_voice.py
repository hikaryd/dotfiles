import fcntl
import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch


class VoiceTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        path = Path(__file__).with_name("agent-voice.py")
        spec = importlib.util.spec_from_file_location("agent_voice", path)
        cls.voice = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.voice)

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.cache = Path(self.temp.name) / "audio"
        p = patch.object(self.voice, "CACHE", self.cache)
        p.start()
        self.addCleanup(p.stop)

    def test_rejects_empty_or_long_text_before_rendering(self):
        with patch.object(self.voice, "_render") as render:
            for text in ("", " \n", "я" * 301):
                with self.subTest(text_length=len(text)):
                    with self.assertRaises(ValueError):
                        self.voice.speak(text)
            render.assert_not_called()
        self.assertFalse(self.cache.exists())

    def test_plays_private_audio_and_removes_it_after_playback(self):
        rendered = []

        def render(text, path):
            rendered.append((text, Path(path)))
            self.assertEqual(os.stat(path).st_mode & 0o777, 0o600)
            Path(path).write_bytes(b"audio fixture")

        def play(args, **kwargs):
            self.assertEqual(args[0], "/usr/bin/afplay")
            self.assertEqual(Path(args[1]).read_bytes(), b"audio fixture")
            self.assertTrue(kwargs["check"])
            self.assertEqual(kwargs["timeout"], 60)
            return subprocess.CompletedProcess(args, 0)

        with patch.object(self.voice, "_render", side_effect=render), patch.object(
            self.voice.subprocess, "run", side_effect=play
        ):
            self.assertEqual(self.voice.speak(" Я позову. "), {"played": True})
        self.assertEqual(rendered[0][0], "Я позову́.")
        self.assertFalse(rendered[0][1].exists())
        self.assertEqual(self.cache.stat().st_mode & 0o777, 0o700)

    def test_preserves_existing_stress_and_other_word_forms(self):
        with patch.object(self.voice, "_render") as render, patch.object(
            self.voice.subprocess, "run"
        ):
            self.voice.speak("Позову́, они позовут, я ПОЗОВУ.")
        self.assertEqual(render.call_args.args[0], "Позову́, они позовут, я ПОЗОВУ́.")

    def test_existing_cache_is_private_before_rendering(self):
        self.cache.mkdir(mode=0o755)
        self.cache.chmod(0o755)

        def render(text, path):
            self.assertEqual(self.cache.stat().st_mode & 0o777, 0o700)

        with patch.object(self.voice, "_render", side_effect=render), patch.object(
            self.voice.subprocess, "run"
        ):
            self.voice.speak("Егор, нужна помощь.")

    def test_playback_failure_cleans_up_without_claiming_success(self):
        with patch.object(self.voice, "_render"), patch.object(
            self.voice.subprocess, "run", side_effect=subprocess.TimeoutExpired("afplay", 60)
        ):
            with self.assertRaises(subprocess.TimeoutExpired):
                self.voice.speak("Егор, нужна помощь.")
        self.assertEqual(list(self.cache.glob("*.wav")), [])

    def test_render_failure_cleans_up_without_playing(self):
        with patch.object(self.voice, "_render", side_effect=RuntimeError("engine failed")), patch.object(
            self.voice.subprocess, "run"
        ) as play:
            with self.assertRaises(RuntimeError):
                self.voice.speak("Егор, нужна помощь.")
            play.assert_not_called()
        self.assertEqual(list(self.cache.glob("*.wav")), [])

    def test_busy_voice_skips_instead_of_overlapping(self):
        self.cache.mkdir(mode=0o700)
        with (self.cache / "speech.lock").open("a") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            with patch.object(self.voice, "_render") as render:
                self.assertEqual(self.voice.speak("Егор, нужна помощь."), {"played": False, "reason": "busy"})
                render.assert_not_called()


if __name__ == "__main__":
    unittest.main()
