import importlib.util
import multiprocessing as mp
import tempfile
import time
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location(
    "pixel_scheduler", Path(__file__).with_name("runtime.py")
)
r = importlib.util.module_from_spec(spec)
# Loaded after implementation exists; tests define the contract first.
if Path(spec.origin).exists():
    spec.loader.exec_module(r)


def hold(root, name, mode, entered, release):
    with r.admission(Path(root), name, mode, 3):
        entered.set()
        release.wait(3)


class Fake:
    def __init__(self):
        self.calls = []

    def cdp(self, method, **params):
        self.calls.append((method, params))
        if method == "Target.getTargets":
            return {"targetInfos": []}
        if method == "Target.closeTarget":
            return {"success": True}
        if method == "Target.createTarget":
            return {"targetId": "owned"}
        if method == "Target.attachToTarget":
            return {"sessionId": "sid"}
        if method == "Runtime.evaluate":
            return {
                "result": {
                    "value": {
                        "ready": "complete",
                        "title": "Title",
                        "url": "https://example.org/",
                        "text": "body",
                    }
                }
            }
        return {}

    def meta(self, req):
        self.calls.append(("meta", req))


class Tests(unittest.TestCase):
    def test_classify(self):
        self.assertEqual(
            r.classify('# pixel:read-v1\n{"url":"https://example.org/"}')[0], "read"
        )
        self.assertEqual(r.classify("# pixel:finish-v1")[0], "finish")
        self.assertEqual(r.classify("print(1)")[0], "legacy")
        for s in [
            '# pixel:read-v1\n{"url":"file:///x"}',
            '# pixel:read-v1\n{"url":"https://x","js":"x"}',
            "# pixel:read-v2",
            '# pixel:read-v1\n{"url":"https://x","max_chars":true}',
        ]:
            with self.assertRaises(ValueError):
                r.classify(s)

    def test_own_cleanup(self):
        with tempfile.TemporaryDirectory() as root:
            f = Fake()
            output = r.read_job(
                f.cdp, f.meta, Path(root) / "job.json", {"url": "https://example.org/"}
            )
            self.assertEqual(output["text"], "body")
            self.assertIn(
                ("Target.closeTarget", {"targetId": "owned", "_response_timeout": 1}),
                f.calls,
            )
            self.assertFalse((Path(root) / "job.json").exists())
            self.assertFalse(any(m == "Target.activateTarget" for m, _ in f.calls))

    def test_attach_failure_cleanup(self):
        with tempfile.TemporaryDirectory() as root:
            f = Fake()

            def cdp(method, **params):
                if method == "Target.attachToTarget":
                    raise RuntimeError("disconnected")
                return f.cdp(method, **params)

            with self.assertRaises(RuntimeError):
                r.read_job(
                    cdp,
                    f.meta,
                    Path(root) / "job.json",
                    {"url": "https://example.org/"},
                )
            self.assertTrue(any(m == "Target.closeTarget" for m, _ in f.calls))

    def test_two_readers_exclude_writer_and_third(self):
        with tempfile.TemporaryDirectory() as root:
            ctx = mp.get_context("fork")
            release = ctx.Event()
            events = [ctx.Event() for _ in range(3)]
            procs = [
                ctx.Process(
                    target=hold, args=(root, str(i), "read", events[i], release)
                )
                for i in range(3)
            ]
            try:
                for p in procs[:2]:
                    p.start()
                self.assertTrue(events[0].wait(2))
                self.assertTrue(events[1].wait(2))
                procs[2].start()
                self.assertFalse(events[2].wait(0.15))
                with (
                    self.assertRaises(TimeoutError),
                    r.admission(Path(root), "writer", "legacy", 0.1),
                ):
                    pass
                release.set()
                self.assertTrue(events[2].wait(2))
            finally:
                release.set()
                for p in procs:
                    if p.pid:
                        p.join(4)

    def test_dead_worker_reaped_live_preserved(self):
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            f = Fake()
            r.save(
                root / "read-dead.json", {"target": "dead", "pid": -1, "start": "old"}
            )
            r.reap(f.cdp, root)
            self.assertFalse((root / "read-dead.json").exists())
            from unittest.mock import patch

            r.save(
                root / "read-live.json", {"target": "live", "pid": 123, "start": "same"}
            )
            with patch.object(r, "identity", return_value="same"):
                r.reap(f.cdp, root)
            self.assertTrue((root / "read-live.json").exists())

    def test_close_failure_retains_record(self):
        with tempfile.TemporaryDirectory() as root:
            path = Path(root) / "read.json"
            r.save(path, {"target": "owned"})

            def cdp(method, **params):
                if method == "Target.closeTarget":
                    raise RuntimeError("lost tunnel")
                if method == "Target.getTargets":
                    raise RuntimeError("lost tunnel")

            with self.assertRaises(RuntimeError):
                r.close_owned(cdp, path, "owned")
            self.assertTrue(path.exists())

    def test_legacy_excludes_readers(self):
        with tempfile.TemporaryDirectory() as root:
            ctx = mp.get_context("fork")
            entered = ctx.Event()
            release = ctx.Event()
            p = ctx.Process(
                target=hold, args=(root, "writer", "legacy", entered, release)
            )
            p.start()
            try:
                self.assertTrue(entered.wait(2))
                with (
                    self.assertRaises(TimeoutError),
                    r.admission(Path(root), "reader", "read", 0.1),
                ):
                    pass
            finally:
                release.set()
                p.join(4)

    def test_unknown_close_response_keeps_record(self):
        with tempfile.TemporaryDirectory() as root:
            path = Path(root) / "read.json"
            r.save(path, {"target": "owned"})
            with self.assertRaises(TypeError):
                r.close_owned(lambda *a, **k: {}, path, "owned")
            self.assertTrue(path.exists())

    def test_inherited_session_lock_reused(self):
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            with open(root / ("session-" + r.key("same") + ".lock"), "a+") as fd:
                r.acquire(fd, time.monotonic() + 1)
                with r.admission(root, "same", "read", 0.1, fd.fileno()):
                    pass
                with (
                    self.assertRaises(FileNotFoundError),
                    r.admission(root, "different", "read", 0.1, fd.fileno()),
                ):
                    pass

    def test_reaper_tolerates_finished_worker_record_race(self):
        from unittest.mock import patch

        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            vanished = root / "read-finished.json"
            with patch.object(Path, "glob", return_value=iter([vanished])):
                r.reap(Fake().cdp, root)

    def test_reaper_does_not_hide_corrupt_record(self):
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            (root / "read-corrupt.json").write_text("not JSON")
            with self.assertRaises(ValueError):
                r.reap(Fake().cdp, root)

    def test_close_ack_still_requires_absence(self):
        from unittest.mock import patch

        with tempfile.TemporaryDirectory() as root:
            path = Path(root) / "record.json"
            r.save(path, {"target": "owned"})

            def cdp(method, **params):
                return (
                    {"success": True}
                    if method == "Target.closeTarget"
                    else {"targetInfos": [{"targetId": "owned"}]}
                )

            with (
                patch.object(r.time, "monotonic", side_effect=[0, 3]),
                self.assertRaises(RuntimeError),
            ):
                r.close_owned(cdp, path, "owned")
            self.assertTrue(path.exists())

    def test_target_list_rejects_malformed_entries(self):
        for response in (
            {},
            {"targetInfos": None},
            {"targetInfos": {}},
            {"targetInfos": [None]},
            {"targetInfos": [{}]},
        ):
            with self.assertRaises((TypeError, RuntimeError)):
                r.target_list(lambda *a, response=response, **k: response)

    def test_legacy_malformed_list_preserves_manifest(self):
        import sys
        import types
        from unittest.mock import patch

        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            path = root / ("legacy-" + r.key("legacy") + ".json")
            r.save(path, {"target": "owned"})
            helpers = types.ModuleType("browser_harness.helpers")
            helpers._send = lambda req: None
            with (
                patch.dict(sys.modules, {"browser_harness.helpers": helpers}),
                patch.dict(r.os.environ, {"BU_NAME": "legacy"}),
                self.assertRaises(TypeError),
            ):
                r.execute(
                    {"cdp": lambda *a, **k: {}}, "print(1)", {"state_dir": str(root)}
                )
            self.assertTrue(path.exists())

    def test_preexisting_legacy_and_finish_do_not_touch_browser(self):
        import io
        from contextlib import redirect_stdout
        from unittest.mock import Mock, patch

        with tempfile.TemporaryDirectory() as root:
            config = {"state_dir": root, "preexisting_sessions": ["existing"]}
            cdp, meta = Mock(), Mock()
            namespace = {"cdp": cdp}
            with patch.dict(r.os.environ, {"BU_NAME": "existing"}):
                r.execute(namespace, "result = 42", config, meta=meta)
                self.assertEqual(namespace["result"], 42)
                output = io.StringIO()
                with redirect_stdout(output):
                    r.execute(namespace, "# pixel:finish-v1", config, meta=meta)
            self.assertIn('"preserved_unmanaged": true', output.getvalue())
            cdp.assert_not_called()
            meta.assert_not_called()

    def test_preexisting_managed_manifest_still_finishes(self):
        import io
        from contextlib import redirect_stdout
        from unittest.mock import Mock, patch

        with tempfile.TemporaryDirectory() as root:
            config = {"state_dir": root, "preexisting_sessions": ["existing"]}
            path = Path(root) / ("legacy-" + r.key("existing") + ".json")
            r.save(path, {"target": "owned"})
            fake, meta = Fake(), Mock()
            with (
                patch.dict(r.os.environ, {"BU_NAME": "existing"}),
                redirect_stdout(io.StringIO()),
            ):
                r.execute({"cdp": fake.cdp}, "# pixel:finish-v1", config, meta=meta)
            self.assertFalse(path.exists())
            meta.assert_called_once_with({"meta": "shutdown"})

    def test_preexisting_config_validation(self):
        for names in ("name", None, [None], [""], ["bad/name"], ["a" * 65]):
            with self.assertRaises(ValueError):
                r.preexisting_sessions({"preexisting_sessions": names})

    def test_same_session_serial(self):
        with tempfile.TemporaryDirectory() as root:
            ctx = mp.get_context("fork")
            entered = ctx.Event()
            release = ctx.Event()
            p = ctx.Process(target=hold, args=(root, "same", "read", entered, release))
            p.start()
            try:
                self.assertTrue(entered.wait(2))
                with (
                    self.assertRaises(TimeoutError),
                    r.admission(Path(root), "same", "read", 0.1),
                ):
                    pass
            finally:
                release.set()
                p.join(4)


if __name__ == "__main__":
    unittest.main()
