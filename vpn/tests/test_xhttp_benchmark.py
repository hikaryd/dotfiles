"""XHTTP conversion, failure evidence, and cleanup of private test processes."""
import argparse
import contextlib
import importlib.util
import io
import json
import os
import subprocess
import tempfile
import time
import unittest
import urllib.parse
from pathlib import Path
from unittest.mock import Mock, patch

spec = importlib.util.spec_from_file_location(
    'xhttp_benchmark', Path(__file__).resolve().parents[1] / 'xhttp-benchmark.py')
benchmark = importlib.util.module_from_spec(spec)
spec.loader.exec_module(benchmark)


def node(query=None):
    uri = urllib.parse.urlsplit('vless://fake-uuid@node.example:443')
    values = {'type': 'xhttp', 'security': 'reality', 'encryption': 'none',
              'flow': '', 'fp': 'chrome', 'sni': 'cover.example', 'pbk': 'fake-key'}
    values.update(query or {})
    return uri, values


def arguments(**changes):
    values = {'source_index': 6, 'bytes': 1000, 'samples': 1, 'timeout': 2,
              'max_duration': 10, 'urls': None, 'check_only': False,
              'install_only': False}
    values.update(changes)
    return argparse.Namespace(**values)


class ConversionTests(unittest.TestCase):
    def test_https_sources_accept_fixed_file_and_template_without_secret_output(self):
        urls = benchmark.validated_urls(
            ['https://speed.example/file?bytes={bytes}', 'https://other.example/10Mb.dat'], 1000)
        self.assertEqual(urls, ['https://speed.example/file?bytes=1000',
                                'https://other.example/10Mb.dat'])

    def test_invalid_source_shapes_and_source_count_are_rejected(self):
        for urls in [['https://user:fake-secret@speed.example/file'],
                     ['https://speed.example/file\n'],
                     ['https://speed.example/file?size={unknown}'],
                     ['http://speed.example/file'],
                     ['https://speed.example/file'] * 5]:
            with self.subTest(urls=urls), self.assertRaises(benchmark.BenchmarkError) as raised:
                benchmark.validated_urls(urls, 1000)
            self.assertNotIn('fake-secret', str(raised.exception))

    def test_reality_xhttp_without_optional_fields_preserves_protocol(self):
        outbound = benchmark.build_outbound(*node())
        self.assertEqual(outbound['settings']['flow'], '')
        stream = outbound['streamSettings']
        self.assertEqual(stream['network'], 'xhttp')
        self.assertEqual(stream['xhttpSettings'], {'mode': 'auto', 'path': '/'})
        self.assertEqual(stream['realitySettings']['password'], 'fake-key')
        self.assertEqual(stream['realitySettings']['shortId'], '')

    def test_percent_decode_once_and_keep_nested_extra(self):
        extra = {'xmux': {'maxConnections': 3},
                 'downloadSettings': {'address': 'down.example', 'port': 443,
                                      'network': 'xhttp', 'security': 'reality'}}
        encoded = urllib.parse.urlencode({'path': '/literal%2F', 'extra': json.dumps(extra)})
        values = dict(urllib.parse.parse_qsl(encoded))
        stream = benchmark.build_outbound(*node(values))['streamSettings']
        self.assertEqual(stream['xhttpSettings']['path'], '/literal%2F')
        self.assertEqual(stream['xhttpSettings']['extra'], extra)

    def test_vision_conflict_and_invalid_extra_are_explicit(self):
        for values, expected in [({'flow': 'xtls-rprx-vision'},
                                  'vision_incompatible_with_xhttp_none'),
                                 ({'extra': '[]'}, 'xhttp_extra_not_object'),
                                 ({'extra': 'fake-secret'}, 'invalid_xhttp_extra_json')]:
            with self.subTest(values=values), self.assertRaisesRegex(
                    benchmark.BenchmarkError, expected):
                benchmark.build_outbound(*node(values))

    def test_source_fetch_exception_never_contains_url_credentials(self):
        with patch.object(benchmark.urllib.request, 'urlopen', side_effect=
                          ValueError('https://fake-token@private.example')), self.assertRaises(
                              benchmark.BenchmarkError) as raised:
            benchmark.read_url('https://fake-token@private.example',
                               time.monotonic() + 10, 1000, 'test')
        self.assertEqual(str(raised.exception), 'source_fetch_failed')
        self.assertIsNone(raised.exception.__cause__)


class TrialTests(unittest.TestCase):
    def measure(self, output, returncode=0, budget=10):
        process = Mock()
        process.communicate.return_value = (output.encode(), None)
        process.returncode = returncode
        with tempfile.TemporaryDirectory() as temporary, patch.object(
                benchmark.subprocess, 'Popen', return_value=process) as popen:
            result = benchmark.trial(12345, ('fake-user', 'fake-password'),
                                     'https://speed.example/file', arguments(),
                                     time.monotonic() + budget, Path(temporary), 1, 1)
        return result, popen.call_args, process

    def test_only_complete_http_200_success_has_throughput(self):
        for output, returncode in [('302\t1000\t0.5\t2000', 0),
                                   ('200\t999\t0.5\t2000', 0),
                                   ('200\t1000\t0.5\t2000', 28),
                                   ('200\t1000\tNaN\t2000', 0),
                                   ('200\t1000\t0.5\t-1', 0),
                                   ('invalid', 0)]:
            with self.subTest(output=output, returncode=returncode):
                result, _, _ = self.measure(output, returncode)
                self.assertFalse(result['success'])
                self.assertIsNone(result['mbps'])
        result, _, _ = self.measure('200\t1000\t0.5\t2000')
        self.assertTrue(result['success'])
        self.assertEqual(result['mbps'], 0.016)

    def test_curl_ignores_config_and_uses_remote_dns_under_overall_deadline(self):
        _, call, process = self.measure('200\t1000\t0.5\t2000', budget=0.5)
        command = call.args[0]
        self.assertEqual(command[:2], ['curl', '--disable'])
        self.assertEqual(command[command.index('--noproxy') + 1], '')
        self.assertEqual(command[command.index('--socks5-hostname') + 1], '127.0.0.1:12345')
        self.assertNotIn('fake-password', ' '.join(command))
        self.assertIn(b'fake-password', process.communicate.call_args.kwargs['input'])
        self.assertLessEqual(process.communicate.call_args.kwargs['timeout'], 0.5)


class CleanupTests(unittest.TestCase):
    def test_process_group_is_stopped_without_touching_parent(self):
        process = subprocess.Popen(['sleep', '60'], start_new_session=True)
        try:
            benchmark.stop_process(process)
            self.assertIsNotNone(process.poll())
            self.assertNotEqual(os.getpid(), process.pid)
        finally:
            if process.poll() is None:
                process.kill()
                process.wait()

    def test_check_only_keeps_config_private_and_removes_it_without_launch(self):
        def check(command, **_kwargs):
            path = Path(command[-1])
            config = json.loads(path.read_text())
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            self.assertEqual(path.parent.stat().st_mode & 0o777, 0o700)
            self.assertEqual(config['inbounds'][0]['listen'], '127.0.0.1')
            self.assertEqual(config['inbounds'][0]['settings']['auth'], 'password')
            self.assertEqual(len(config['outbounds']), 1)
            self.assertEqual(config['outbounds'][0]['protocol'], 'vless')
            return subprocess.CompletedProcess(command, 0, b'Configuration OK', b'')

        output = io.StringIO()
        with tempfile.TemporaryDirectory() as temporary, patch.object(
                benchmark, 'BASE', Path(temporary)), patch.object(
                benchmark, 'ensure_binary', return_value=Path('/fake/xray')), patch.object(
                benchmark, 'load_node', return_value=node()), patch.object(
                benchmark.subprocess, 'run', side_effect=check), patch.object(
                benchmark.subprocess, 'Popen') as popen, contextlib.redirect_stdout(output):
            result = benchmark.benchmark(arguments(check_only=True), time.monotonic() + 10)
            self.assertEqual(list(Path(temporary).iterdir()), [])
        self.assertEqual(result, 0)
        popen.assert_not_called()
        self.assertNotIn('fake-key', output.getvalue())
        self.assertNotIn('fake-uuid', output.getvalue())
        self.assertFalse(json.loads(output.getvalue())['handshake_tested'])

    def test_start_failure_removes_private_config_and_logs(self):
        process = Mock()
        process.poll.return_value = 1
        with tempfile.TemporaryDirectory() as temporary, patch.object(
                benchmark, 'BASE', Path(temporary)), patch.object(
                benchmark, 'ensure_binary', return_value=Path('/fake/xray')), patch.object(
                benchmark, 'load_node', return_value=node()), patch.object(
                benchmark.subprocess, 'run', return_value=subprocess.CompletedProcess(
                    [], 0, b'Configuration OK', b'')), patch.object(
                benchmark.subprocess, 'Popen', return_value=process), contextlib.redirect_stdout(
                    io.StringIO()):
            with self.assertRaisesRegex(benchmark.BenchmarkError, 'xray_exited_before_listening'):
                benchmark.benchmark(arguments(), time.monotonic() + 10)
            self.assertEqual(list(Path(temporary).iterdir()), [])


if __name__ == '__main__':
    unittest.main()
