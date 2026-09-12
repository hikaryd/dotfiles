"""Exec-boundary tests: a real subprocess with a dependency-free fake upstream."""

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

BASE = Path(__file__).parent
UPSTREAM = """import ast, fcntl, hashlib, json, os, pathlib, sys
source = sys.stdin.read()
fd = os.environ.get("PIXEL_SESSION_LOCK_FD")
result = {"source": source, "locked": False}
if fd is not None:
    fd = int(fd)
    config = json.loads(pathlib.Path(os.environ["PIXEL_SCHEDULER_CONFIG"]).read_text())
    name = os.environ["BU_NAME"]
    path = pathlib.Path(config["state_dir"]) / ("session-" + hashlib.sha256(name.encode()).hexdigest() + ".lock")
    result["same_inode"] = os.fstat(fd).st_ino == path.stat().st_ino
    with path.open("a+") as other:
        try:
            fcntl.flock(other, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            result["locked"] = True
    call = ast.parse(source).body[-1].value
    result["payload"] = ast.literal_eval(call.args[1])
print(json.dumps(result))
"""


class ShimTests(unittest.TestCase):
    def run_shim(self, source, *, extra=None, endpoints=None, direct_stub=False):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            upstream = root / "upstream.py"
            upstream.write_text(UPSTREAM)
            preamble = root / "preamble.txt"
            preamble.write_text("# exact own tab preamble\n")
            runtime = BASE / "runtime.py"
            if direct_stub:
                runtime = root / "runtime.py"
                runtime.write_text(
                    (BASE / "runtime.py").read_text()
                    + '\ndef direct_read(source, config):\n    print(json.dumps({"direct": True, "source": source, "pid": os.getpid()}))\n'
                )
            config = root / "config.json"
            config.write_text(
                json.dumps(
                    {
                        "upstream_argv": [sys.executable, str(upstream)],
                        "runtime_file": str(runtime),
                        "read_python": sys.executable,
                        "preamble_file": str(preamble),
                        "state_dir": str(root / "state"),
                        "pixel_cdp_urls": ["http://127.0.0.1:19222"]
                        if endpoints is None
                        else endpoints,
                    }
                )
            )
            env = {
                k: v
                for k, v in os.environ.items()
                if k not in ("BU_CDP_URL", "BU_CDP_WS", "PIXEL_SESSION_LOCK_FD")
            }
            env.update(
                PIXEL_SCHEDULER_CONFIG=str(config),
                BU_NAME="test_job",
                BU_CDP_URL="http://127.0.0.1:19222",
            )
            env.update(extra or {})
            return subprocess.run(
                [sys.executable, str(BASE / "shim.py")],
                check=False,
                input=source,
                text=True,
                capture_output=True,
                env=env,
                timeout=5,
            )

    def test_exact_preamble_and_inherited_lock(self):
        result = self.run_shim("# exact own tab preamble\nprint(1)")
        self.assertEqual(result.returncode, 0, result.stderr)
        data = json.loads(result.stdout)
        self.assertEqual(data["payload"], "print(1)")
        self.assertTrue(data["locked"])
        self.assertTrue(data["same_inode"])

    def test_missing_changed_and_invalid_directive_rejected(self):
        for source in (
            "print(1)",
            "# different preamble\nprint(1)",
            "# exact own tab preamble\n# pixel:read-v9",
        ):
            result = self.run_shim(source)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(result.stdout, "")

    def test_ambiguous_endpoints_rejected(self):
        for values in (
            {"BU_CDP_WS": "ws://other/"},
            {"BU_CDP_URL": "http://other/", "BU_CDP_WS": "http://127.0.0.1:19222"},
        ):
            result = self.run_shim("print(1)", extra=values)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(result.stdout, "")

    def test_nonpixel_passthrough(self):
        result = self.run_shim("unchanged", extra={"BU_CDP_URL": "http://other/"})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["source"], "unchanged")

    def test_invalid_endpoint_config(self):
        for endpoints in ([], [""], "http://127.0.0.1:19222", [123]):
            result = self.run_shim("print(1)", endpoints=endpoints)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(result.stdout, "")

    def test_read_route_bypasses_upstream(self):
        source = '# pixel:read-v1\n{"url":"https://example.org"}'
        result = self.run_shim("# exact own tab preamble\n" + source, direct_stub=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        data = json.loads(result.stdout)
        self.assertTrue(data["direct"])
        self.assertEqual(data["source"], source)

    def test_unmatched_pixel_directives_never_reach_upstream(self):
        for source in (
            "# pixel:finish-v1",
            '# exact own tab preamble\n# pixel:read-v1\n{"url":"https://example.org"}',
        ):
            result = self.run_shim(source, extra={"BU_CDP_URL": "http://other/"})
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(result.stdout, "")

    def test_explicit_websocket_route_reaches_direct_read(self):
        endpoint = "ws://127.0.0.1:19222/devtools/browser"
        result = self.run_shim(
            '# exact own tab preamble\n# pixel:read-v1\n{"url":"https://example.org"}',
            extra={"BU_CDP_URL": "", "BU_CDP_WS": endpoint},
            endpoints=[endpoint],
            direct_stub=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(json.loads(result.stdout)["direct"])
