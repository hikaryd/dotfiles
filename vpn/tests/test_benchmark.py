"""Checks that failed probes never become throughput and isolated processes exit."""
import contextlib
import importlib.util
import io
import json
import os
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path
from unittest.mock import patch

spec = importlib.util.spec_from_file_location(
    "vpn_benchmark", Path(__file__).resolve().parents[1] / "benchmark.py")
benchmark = importlib.util.module_from_spec(spec)
spec.loader.exec_module(benchmark)


class MeasurementTests(unittest.TestCase):
    def curl_result(self, **changes):
        data = {"http_code": 200, "size_download": 1000, "speed_download": 2000000,
                "time_starttransfer": 0.02, "time_total": 0.5}
        data.update(changes)
        return subprocess.CompletedProcess([], 0, json.dumps(data), "")

    def test_http_error_or_partial_body_has_no_throughput(self):
        for changes, error in [({"http_code": 403}, "unexpected_http"),
                               ({"http_code": 302}, "unexpected_http"),
                               ({"size_download": 999}, "incomplete_download")]:
            with self.subTest(changes=changes), patch.object(
                    benchmark.subprocess, "run", return_value=self.curl_result(**changes)):
                trial = benchmark.measurement(1234, "fake-password", 1000, 1)
                self.assertFalse(trial["ok"])
                self.assertIsNone(trial["mbps"])
                self.assertEqual(trial["error"], error)

    def test_transport_failure_or_invalid_metrics_has_no_throughput(self):
        results = [subprocess.CompletedProcess([], 28, self.curl_result().stdout, ""),
                   self.curl_result(speed_download=float("nan")),
                   self.curl_result(time_total=-1),
                   subprocess.CompletedProcess([], 0, "{}", ""),
                   subprocess.CompletedProcess([], 0, "not JSON", "")]
        for result in results:
            with self.subTest(result=result.stdout), patch.object(
                    benchmark.subprocess, "run", return_value=result):
                trial = benchmark.measurement(1234, "fake-password", 1000, 1)
                self.assertFalse(trial["ok"])
                self.assertIsNone(trial["mbps"])

    def test_socks_password_is_not_in_argv_and_source_size_is_explicit(self):
        with patch.object(benchmark.subprocess, "run", return_value=self.curl_result()) as run:
            trial = benchmark.measurement(
                1234, "fake-password", 1000, 1, "https://other.example/file?size={bytes}")
        self.assertTrue(trial["ok"])
        self.assertEqual(trial["mbps"], 16)
        self.assertNotIn("fake-password", " ".join(run.call_args.args[0]))
        self.assertIn("fake-password", run.call_args.kwargs["input"])
        self.assertEqual(run.call_args.args[0][1], "--disable")
        self.assertIn("https://other.example/file?size=1000", run.call_args.args[0])

    def test_remaining_overall_budget_bounds_the_active_curl_process(self):
        with patch.object(benchmark.subprocess, "run", side_effect=
                          subprocess.TimeoutExpired(["curl"], 0.5)) as run:
            trial = benchmark.measurement(1234, "fake-password", 1000, 20,
                                          deadline=time.monotonic() + 0.5)
        self.assertFalse(trial["ok"])
        self.assertLessEqual(run.call_args.kwargs["timeout"], 0.5)
        self.assertGreater(run.call_args.kwargs["timeout"], 0)


class ConfigurationTests(unittest.TestCase):
    def test_unsafe_url_is_rejected_without_echoing_credentials(self):
        for url in ("https://alice:fake-secret@example.com/file", "file:///etc/passwd",
                    "https://example.com:99999/file", "https://example.com/file\nheader",
                    "https://example.com/{unknown}", "https://example.com/#fragment"):
            with self.subTest(url=url), self.assertRaises(ValueError) as raised:
                benchmark.download_url(url, 1000)
            self.assertNotIn("fake-secret", str(raised.exception))
            self.assertNotIn(url, str(raised.exception))

    def test_only_manual_tag_does_not_require_an_auto_selector(self):
        config = {"outbounds": [{"tag": "manual", "type": "vless", "uuid": "fake-uuid"}]}
        candidates = benchmark.select_candidates(config, ["manual"], [])
        self.assertEqual([n["tag"] for n in candidates], ["VPS-direct", "manual"])
        with self.assertRaises(ValueError):
            benchmark.select_candidates(config, ["missing"], [])

    def test_all_scans_more_than_sixteen_proxy_nodes_and_excludes_helpers(self):
        proxies = [{"type": protocol, "tag": f"node-{i}", "password": "fake-secret"}
                   for i, protocol in enumerate(
                       ["vless", "hysteria2", "hysteria", "trojan", "shadowsocks", "vmess"] * 3)]
        helpers = [{"type": kind, "tag": kind} for kind in
                   ("direct", "block", "dns", "bridge", "selector", "urltest")]
        config = {"outbounds": proxies + helpers}
        candidates = benchmark.select_candidates(config, [], [], all_nodes=True)
        self.assertGreater(len(candidates), 17)
        self.assertEqual(candidates[1:], proxies)
        self.assertEqual(candidates[0]["tag"], "VPS-direct")
        for proxy in proxies:
            with self.subTest(protocol=proxy["type"]):
                selected = benchmark.select_candidates(config, [proxy["tag"]], [])
                self.assertEqual(selected[1], proxy)

    def test_default_keeps_auto_pool_without_expanding_to_all_proxies(self):
        config = {"outbounds": [
            {"type": "hysteria2", "tag": "finland"},
            {"type": "trojan", "tag": "moscow"},
            {"type": "urltest", "tag": "auto", "outbounds": ["finland"]}]}
        selected = benchmark.select_candidates(config, [], [])
        self.assertEqual([node["tag"] for node in selected], ["VPS-direct", "finland"])

    def test_screening_defaults_and_exhausted_budget_report_every_skipped_node(self):
        config = {"outbounds": [
            {"type": "hysteria2", "tag": "finland", "password": "fake-secret"},
            {"type": "trojan", "tag": "moscow", "password": "fake-secret"}]}
        output = io.StringIO()

        def slow_probe(*_args):
            time.sleep(0.02)
            return [{"ok": True, "mbps": 16}]

        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "config.json"
            path.write_text(json.dumps(config), encoding="utf-8")
            with patch.object(benchmark, "benchmark", side_effect=slow_probe) as probe, \
                    contextlib.redirect_stdout(output):
                status = benchmark.main(["--config", str(path), "--all", "--max-duration", "0.01"])
        self.assertEqual(status, 1)
        self.assertEqual(probe.call_count, 1)
        self.assertEqual(probe.call_args.args[2:4], (1_000_000, 10))
        rows = [json.loads(line) for line in output.getvalue().splitlines()]
        plan, complete = rows[0], rows[-1]
        self.assertEqual(plan["samples"], 1)
        self.assertEqual(plan["total_trials"], 3)
        self.assertEqual(complete["attempted"], 1)
        self.assertEqual(complete["skipped"], 2)
        trials = [row for row in rows if row["event"] == "trial"]
        self.assertEqual([row["protocol"] for row in trials], ["direct", "hysteria2", "trojan"])
        self.assertTrue(all(row["trial"]["error"] == "duration_budget_exhausted"
                            for row in trials[1:]))
        self.assertNotIn("fake-secret", output.getvalue())

    def test_sources_are_reported_separately_and_failed_candidates_exit_nonzero(self):
        config = {"outbounds": [{"tag": "manual", "type": "vless", "uuid": "fake-uuid"}]}
        success = {"ok": True, "mbps": 16}
        failure = benchmark.failed_trial("curl_failed")
        output = io.StringIO()
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "config.json"
            path.write_text(json.dumps(config), encoding="utf-8")
            with patch.object(benchmark, "benchmark", side_effect=[
                    [success], [success], [success], [failure]]), contextlib.redirect_stdout(output):
                status = benchmark.main([
                    "--config", str(path), "--only", "manual", "--samples", "1",
                    "--url", benchmark.DEFAULT_URL,
                    "--url", "https://other.example/file?token=fake-secret&bytes={bytes}"])
        self.assertEqual(status, 1)
        self.assertNotIn("fake-secret", output.getvalue())
        rows = [json.loads(line) for line in output.getvalue().splitlines()]
        summaries = [row for row in rows if "median_mbps" in row]
        self.assertEqual(len(summaries), 4)
        self.assertEqual({row["source"] for row in summaries},
                         {"1:speed.cloudflare.com", "2:other.example"})
        self.assertIsNone(summaries[-1]["median_mbps"])


class CleanupTests(unittest.TestCase):
    def test_real_child_and_secret_config_are_removed_on_exception_and_cancel(self):
        # A real local listener exercises OS process cleanup; no network downloads.
        child = """
import json, socket, sys
config = json.load(open(sys.argv[1]))
sock = socket.socket()
sock.bind(('127.0.0.1', config['inbounds'][0]['listen_port']))
sock.listen()
while True:
    conn, addr = sock.accept()
    conn.close()
"""
        real_popen = subprocess.Popen
        for exception in (RuntimeError("probe failed"), KeyboardInterrupt()):
            processes, paths = [], []

            def start(args, paths=paths, processes=processes, **kwargs):
                paths.append(Path(args[-1]))
                self.assertEqual(paths[-1].stat().st_mode & 0o777, 0o600)
                proc = real_popen([sys.executable, "-c", child, args[-1]], **kwargs)
                processes.append(proc)
                return proc

            with self.subTest(exception=type(exception).__name__), patch.object(
                    benchmark.subprocess, "Popen", side_effect=start), patch.object(
                    benchmark, "measurement", side_effect=exception):
                try:
                    with self.assertRaises(type(exception)):
                        benchmark.benchmark({"type": "direct", "tag": "direct"}, 1, 1000, 1)
                    self.assertIsNotNone(processes[0].poll())
                    self.assertFalse(paths[0].parent.exists())
                finally:
                    for proc in processes:
                        if proc.poll() is None:
                            proc.kill()
                            proc.wait(timeout=5)

    def test_child_ignoring_sigterm_is_killed_and_reaped(self):
        child = """
import signal, sys, time
signal.signal(signal.SIGTERM, signal.SIG_IGN)
print('ready', flush=True)
while True:
    time.sleep(1)
"""
        with subprocess.Popen([sys.executable, "-c", child], stdout=subprocess.PIPE,
                              text=True) as proc:
            try:
                self.assertEqual(proc.stdout.readline().strip(), "ready")
                with patch.object(benchmark, "STOP_TIMEOUT", 0.05):
                    benchmark.stop_process(proc)
                self.assertIsNotNone(proc.poll())
                with self.assertRaises(ProcessLookupError):
                    os.kill(proc.pid, 0)
            finally:
                if proc.poll() is None:
                    proc.kill()
                    proc.wait(timeout=5)


if __name__ == "__main__":
    unittest.main()
