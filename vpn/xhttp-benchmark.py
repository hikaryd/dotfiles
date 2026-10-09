#!/usr/bin/env python3
"""Isolated Xray XHTTP benchmark. Does not alter live VPN services or routes."""
import argparse
import hashlib
import http.client
import io
import json
import math
import os
import platform
import re
import secrets
import signal
import socket
import statistics
import subprocess
import sys
import tempfile
import time
import urllib.parse
import urllib.request
import zipfile
from pathlib import Path

BASE = Path(__file__).resolve().parent
VERSION = '26.3.27'
ASSET_URL = ('https://github.com/XTLS/Xray-core/releases/download/v26.3.27/'
             'Xray-linux-64.zip')
ASSET_SHA256 = '23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae'
DEFAULT_URL = 'https://speed.cloudflare.com/__down?bytes={bytes}'


class BenchmarkError(Exception):
    """Static, safe error code; no URI or credential values."""


def emit(event, **fields):
    print(json.dumps({'event': event, **fields}, ensure_ascii=False), flush=True)


def remaining(deadline):
    value = deadline - time.monotonic()
    if value <= 0:
        raise BenchmarkError('deadline_exceeded')
    return value


def read_url(url, deadline, maximum, user_agent):
    request = urllib.request.Request(url, headers={'User-Agent': user_agent})
    chunks = []
    total = 0
    try:
        with urllib.request.urlopen(request, timeout=min(30, remaining(deadline))) as response:
            if response.status != 200:
                raise BenchmarkError('source_http_status')
            while True:
                remaining(deadline)
                chunk = response.read(1024 * 1024)
                if not chunk:
                    break
                total += len(chunk)
                if total > maximum:
                    raise BenchmarkError('source_too_large')
                chunks.append(chunk)
    except BenchmarkError:
        raise
    except (OSError, ValueError, http.client.HTTPException):
        raise BenchmarkError('source_fetch_failed') from None
    return b''.join(chunks)


def ensure_binary(deadline):
    if platform.system() != 'Linux' or platform.machine() not in ('x86_64', 'amd64'):
        raise BenchmarkError('linux_x64_required')
    directory = BASE / 'tools' / ('xray-' + VERSION)
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    if directory.is_symlink():
        raise BenchmarkError('unsafe_binary_directory')
    archive = directory / 'Xray-linux-64.zip'
    if archive.exists():
        if archive.is_symlink():
            raise BenchmarkError('unsafe_archive_path')
        data = archive.read_bytes()
    else:
        data = read_url(ASSET_URL, deadline, 30 * 1024 * 1024, 'isolated-xhttp-diagnostics')
    if hashlib.sha256(data).hexdigest() != ASSET_SHA256:
        raise BenchmarkError('archive_sha256_mismatch')
    if not archive.exists():
        with archive.open('xb') as output:
            os.chmod(archive, 0o600)
            output.write(data)
    try:
        with zipfile.ZipFile(io.BytesIO(data)) as package:
            info = package.getinfo('xray')
            if info.file_size > 100 * 1024 * 1024:
                raise BenchmarkError('binary_too_large')
            binary_data = package.read(info)
    except BenchmarkError:
        raise
    except (zipfile.BadZipFile, KeyError, OSError, RuntimeError):
        raise BenchmarkError('archive_invalid') from None
    binary = directory / 'xray'
    if binary.is_symlink():
        raise BenchmarkError('unsafe_binary_path')
    if not binary.exists() or binary.read_bytes() != binary_data:
        temporary = None
        try:
            with tempfile.NamedTemporaryFile(prefix='.xray-install-', dir=directory,
                                             delete=False) as output:
                temporary = Path(output.name)
                os.chmod(temporary, 0o700)
                output.write(binary_data)
            os.replace(temporary, binary)
        finally:
            if temporary is not None:
                temporary.unlink(missing_ok=True)
    os.chmod(binary, 0o700)
    try:
        result = subprocess.run([str(binary), 'version'], capture_output=True,
                                timeout=min(5, remaining(deadline)), check=False)
    except (OSError, subprocess.TimeoutExpired):
        raise BenchmarkError('binary_version_failed') from None
    if result.returncode != 0 or not re.search(rb'Xray 26\.3\.27(?:\s|$)', result.stdout):
        raise BenchmarkError('binary_version_mismatch')
    return binary


def load_node(index, deadline):
    try:
        values = {}
        with open('/etc/sing-box/vpn.env', encoding='utf-8') as source:
            for line in source:
                line = line.strip()
                if line and not line.startswith('#') and '=' in line:
                    key, value = line.split('=', 1)
                    values[key.strip()] = value.strip().strip('"').strip("'")
        source_url = values['SUB_URL']
        raw = read_url(source_url, deadline, 5 * 1024 * 1024, 'sing-box')
        line = raw.decode('utf-8', 'strict').splitlines()[index].strip()
        uri = urllib.parse.urlsplit(line)
        pairs = urllib.parse.parse_qsl(uri.query, keep_blank_values=True)
        query = dict(pairs)
        if len(query) != len(pairs):
            raise BenchmarkError('duplicate_query_parameters')
        if uri.scheme != 'vless' or query.get('type') not in ('xhttp', 'splithttp'):
            raise BenchmarkError('source_node_not_vless_xhttp')
        if not uri.hostname or not uri.username or uri.password is not None:
            raise BenchmarkError('invalid_vless_authority')
        port = uri.port
        if port is None or not 1 <= port <= 65535:
            raise BenchmarkError('invalid_vless_port')
        return uri, query
    except BenchmarkError:
        raise
    except (OSError, KeyError, IndexError, ValueError):
        raise BenchmarkError('source_node_parse_failed') from None


def build_outbound(uri, query):
    encryption = query.get('encryption', 'none')
    flow = query.get('flow', '')
    if not encryption:
        raise BenchmarkError('empty_vless_encryption')
    if flow not in ('', 'xtls-rprx-vision', 'xtls-rprx-vision-udp443'):
        raise BenchmarkError('unsupported_vless_flow')
    if flow and encryption == 'none':
        raise BenchmarkError('vision_incompatible_with_xhttp_none')
    mode = query.get('mode', 'auto')
    if mode not in ('auto', 'packet-up', 'stream-up', 'stream-one'):
        raise BenchmarkError('invalid_xhttp_mode')
    path = query.get('path', '/')
    if not path:
        raise BenchmarkError('empty_xhttp_path')
    xhttp = {'mode': mode, 'path': path}
    if 'host' in query:
        if not query['host']:
            raise BenchmarkError('empty_xhttp_host')
        xhttp['host'] = query['host']
    if 'extra' in query:
        try:
            extra = json.loads(query['extra'])
        except json.JSONDecodeError:
            raise BenchmarkError('invalid_xhttp_extra_json') from None
        if not isinstance(extra, dict):
            raise BenchmarkError('xhttp_extra_not_object')
        xhttp['extra'] = extra
    security = query.get('security', 'none')
    stream = {'network': 'xhttp', 'security': security, 'xhttpSettings': xhttp}
    if security == 'reality':
        if not query.get('pbk') or not query.get('fp'):
            raise BenchmarkError('missing_reality_password_or_fingerprint')
        reality = {'serverName': query.get('sni', uri.hostname),
                   'fingerprint': query['fp'], 'password': query['pbk'],
                   'shortId': query.get('sid', '')}
        if not reality['serverName']:
            raise BenchmarkError('empty_reality_server_name')
        if 'spx' in query:
            reality['spiderX'] = query['spx']
        if 'pqv' in query:
            reality['mldsa65Verify'] = query['pqv']
        stream['realitySettings'] = reality
    elif security == 'tls':
        tls = {'serverName': query.get('sni', uri.hostname),
               'fingerprint': query.get('fp', 'chrome')}
        if 'alpn' in query:
            tls['alpn'] = query['alpn'].split(',')
        for uri_key, config_key in (('ech', 'echConfigList'),
                                    ('pcs', 'pinnedPeerCertSha256'),
                                    ('vcn', 'verifyPeerCertByName')):
            if uri_key in query:
                tls[config_key] = query[uri_key]
        stream['tlsSettings'] = tls
    elif security != 'none':
        raise BenchmarkError('unsupported_transport_security')
    return {'tag': 'xhttp-source-node', 'protocol': 'vless',
            'settings': {'address': uri.hostname, 'port': uri.port,
                         'id': urllib.parse.unquote(uri.username),
                         'encryption': encryption, 'flow': flow},
            'streamSettings': stream}


def reserve_port():
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(('127.0.0.1', 0))
        return listener.getsockname()[1]


def stop_process(process):
    if process is None or process.poll() is not None:
        return
    try:
        os.killpg(process.pid, signal.SIGTERM)
        process.wait(timeout=3)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        process.wait(timeout=3)
    except ProcessLookupError:
        process.wait(timeout=3)


def trial(port, auth, url, args, deadline, directory, sample, source_id):
    timeout = min(args.timeout, remaining(deadline))
    error_file = directory / ('curl-' + str(sample) + '-' + str(source_id) + '.log')
    command = ['curl', '--disable', '--config', '-', '--silent', '--show-error',
               '--fail', '--retry', '0',
               '--noproxy', '', '--socks5-hostname', '127.0.0.1:' + str(port),
               '--proto', '=https', '--proto-redir', '=https',
               '--connect-timeout', str(min(10, timeout)), '--max-time', str(timeout),
               '--output', '/dev/null', '--write-out',
               '%{http_code}\t%{size_download}\t%{time_total}\t%{speed_download}\n', url]
    started = time.monotonic()
    process = None
    returncode = None
    output = b''
    error = None
    # Runtime-generated hex credentials need no curl-config quoting escapes.
    proxy_config = ('proxy-user = "' + auth[0] + ':' + auth[1] + '"\n').encode()
    with error_file.open('xb') as errors:
        os.chmod(error_file, 0o600)
        try:
            process = subprocess.Popen(command, stdin=subprocess.PIPE,
                                       stdout=subprocess.PIPE, stderr=errors,
                                       start_new_session=True)
            output, _ = process.communicate(input=proxy_config,
                                            timeout=min(timeout + 1, remaining(deadline)))
            returncode = process.returncode
        except subprocess.TimeoutExpired:
            error = 'trial_timeout'
            stop_process(process)
            output, _ = process.communicate(timeout=1)
            returncode = process.returncode
        except BenchmarkError:
            error = 'deadline_exceeded'
            stop_process(process)
        except (OSError, ValueError):
            error = 'curl_process_failed'
            stop_process(process)
        finally:
            stop_process(process)
    elapsed = time.monotonic() - started
    status = 0
    size = 0
    transfer_time = None
    speed = None
    try:
        fields = output.decode('ascii').strip().split('\t')
        status = int(fields[0])
        size_number = float(fields[1])
        if not math.isfinite(size_number) or size_number < 0 or size_number != int(size_number):
            raise ValueError()
        size = int(size_number)
        transfer_time = float(fields[2])
        speed = float(fields[3])
        if (not math.isfinite(transfer_time) or not math.isfinite(speed)
                or transfer_time <= 0 or speed <= 0):
            raise ValueError()
    except (ValueError, IndexError, UnicodeDecodeError):
        error = error or 'invalid_curl_metrics'
    if error is None:
        if returncode != 0:
            error = 'curl_request_failed'
        elif status != 200:
            error = 'unexpected_http_status'
        elif size != args.bytes:
            error = 'unexpected_body_size'
    success = error is None
    return {'sample': sample, 'source_id': source_id,
            'success': success, 'http_status': status,
            'bytes': size, 'expected_bytes': args.bytes, 'curl_exit': returncode,
            'wall_seconds': round(elapsed, 6), 'transfer_seconds': transfer_time,
            'speed_bytes_per_second': speed if success else None,
            'mbps': round(size * 8 / transfer_time / 1_000_000, 6) if success else None,
            'error': error}


def validated_urls(templates, expected_bytes):
    urls = []
    templates = templates or [DEFAULT_URL]
    if not 1 <= len(templates) <= 4:
        raise BenchmarkError('invalid_source_count')
    for template in templates:
        url = template.replace('{bytes}', str(expected_bytes))
        if any(character.isspace() or ord(character) <= 32 or ord(character) == 127
               for character in url) or any(character in url for character in '{}'):
            raise BenchmarkError('invalid_https_url_template')
        try:
            target = urllib.parse.urlsplit(url)
            valid = (template.count('{bytes}') <= 1 and target.scheme == 'https'
                     and target.hostname and target.username is None and not target.fragment)
            if target.port is not None and not 1 <= target.port <= 65535:
                valid = False
        except ValueError:
            raise BenchmarkError('invalid_https_url_template') from None
        if not valid:
            raise BenchmarkError('invalid_https_url_template')
        urls.append(url)
    return urls


def benchmark(args, deadline):
    urls = validated_urls(args.urls, args.bytes)
    binary = ensure_binary(deadline)
    if args.install_only:
        emit('installed', version=VERSION, archive_sha256=ASSET_SHA256, binary=str(binary))
        return 0
    uri, query = load_node(args.source_index, deadline)
    outbound = build_outbound(uri, query)
    if 'xhttpSettings' not in outbound['streamSettings']:
        raise BenchmarkError('xhttp_configuration_missing')
    results = []
    with tempfile.TemporaryDirectory(prefix='.xhttp-private-', dir=BASE) as temporary:
        directory = Path(temporary)
        port = reserve_port()
        auth = (secrets.token_hex(16), secrets.token_hex(16))
        config = {'log': {'access': 'none', 'error': str(directory / 'xray.log'),
                          'loglevel': 'warning'},
                  'inbounds': [{'tag': 'bench-socks', 'listen': '127.0.0.1',
                                'port': port, 'protocol': 'socks',
                                'settings': {'auth': 'password', 'udp': False,
                                             'accounts': [{'user': auth[0],
                                                           'pass': auth[1]}]}}],
                  'outbounds': [outbound]}
        config_path = directory / 'config.json'
        with config_path.open('x', encoding='utf-8') as output:
            os.chmod(config_path, 0o600)
            json.dump(config, output)
        try:
            checked = subprocess.run([str(binary), 'run', '-test', '-c', str(config_path)],
                                     capture_output=True,
                                     timeout=min(10, remaining(deadline)), check=False)
        except (OSError, subprocess.TimeoutExpired):
            raise BenchmarkError('xray_config_check_failed') from None
        if checked.returncode != 0:
            raise BenchmarkError('xray_config_invalid')
        if args.check_only:
            emit('checked', source_index=args.source_index, version=VERSION,
                 protocol='vless', transport='xhttp', security=query.get('security', 'none'),
                 mode=query.get('mode', 'auto'), handshake_tested=False)
            return 0
        emit('plan', source_index=args.source_index, version=VERSION, protocol='vless',
             transport='xhttp', samples=args.samples, bytes_per_sample=args.bytes,
             sources=len(urls), trials=args.samples * len(urls),
             timeout_seconds=args.timeout, max_duration_seconds=args.max_duration)
        process = None
        with (directory / 'process.log').open('xb') as logs:
            os.chmod(directory / 'process.log', 0o600)
            try:
                process = subprocess.Popen([str(binary), 'run', '-c', str(config_path)],
                                           stdout=logs, stderr=logs, start_new_session=True)
                startup_deadline = min(deadline, time.monotonic() + 8)
                while True:
                    if process.poll() is not None:
                        raise BenchmarkError('xray_exited_before_listening')
                    try:
                        with socket.create_connection(('127.0.0.1', port), timeout=0.2):
                            break
                    except OSError:
                        if time.monotonic() >= startup_deadline:
                            raise BenchmarkError('xray_startup_timeout')
                        time.sleep(0.05)
                for sample in range(1, args.samples + 1):
                    if time.monotonic() >= deadline:
                        break
                    for source_id, url in enumerate(urls, 1):
                        if time.monotonic() >= deadline:
                            break
                        if process.poll() is not None:
                            raise BenchmarkError('xray_exited_during_benchmark')
                        result = trial(port, auth, url, args, deadline,
                                       directory, sample, source_id)
                        results.append(result)
                        emit('trial', source_index=args.source_index, **result)
            finally:
                stop_process(process)
    successful = [x['mbps'] for x in results if x['success']]
    expected_trials = args.samples * len(urls)
    skipped = expected_trials - len(results)
    failures = len(results) - len(successful)
    emit('summary', source_index=args.source_index, completed=len(results),
         successful=len(successful), failed=failures, skipped=skipped,
         median_mbps=statistics.median(successful) if successful else None,
         min_mbps=min(successful) if successful else None,
         max_mbps=max(successful) if successful else None)
    for source_id in range(1, len(urls) + 1):
        selected = [result for result in results if result['source_id'] == source_id]
        speeds = [result['mbps'] for result in selected if result['success']]
        emit('source_summary', source_index=args.source_index, source_id=source_id,
             completed=len(selected), successful=len(speeds),
             failed=len(selected) - len(speeds), skipped=args.samples - len(selected),
             median_mbps=statistics.median(speeds) if speeds else None,
             min_mbps=min(speeds) if speeds else None,
             max_mbps=max(speeds) if speeds else None)
    return 0 if len(successful) == expected_trials else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-index', type=int, default=6)
    parser.add_argument('--bytes', type=int, default=10_000_000)
    parser.add_argument('--samples', type=int, default=3)
    parser.add_argument('--timeout', type=float, default=20)
    parser.add_argument('--max-duration', type=float, default=240)
    parser.add_argument('--url', '--url-template', action='append', dest='urls',
                        help='Repeatable HTTPS URL; optional {bytes} placeholder.')
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument('--install-only', action='store_true')
    mode.add_argument('--check-only', action='store_true')
    args = parser.parse_args()
    if (args.source_index < 0 or not 1 <= args.bytes <= 50_000_000
            or not 1 <= args.samples <= 5 or not math.isfinite(args.timeout)
            or not math.isfinite(args.max_duration) or not 0 < args.timeout <= 30
            or not 0 < args.max_duration <= 86400):
        parser.error('invalid positive limits or source index')
    os.umask(0o077)
    def interrupted(_signal, _frame):
        raise KeyboardInterrupt()
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGHUP, interrupted)
    started = time.monotonic()
    exitcode = 1
    try:
        exitcode = benchmark(args, started + args.max_duration)
    except BenchmarkError as error:
        emit('error', source_index=args.source_index, error=str(error))
    except KeyboardInterrupt:
        emit('error', source_index=args.source_index, error='interrupted')
    except Exception as error:  # noqa: BLE001 - sanitize unexpected errors, exit with failure.
        emit('error', source_index=args.source_index, error='unexpected_error',
             error_type=type(error).__name__)
    emit('complete', source_index=args.source_index, exit_code=exitcode,
         elapsed_seconds=round(time.monotonic() - started, 6))
    return exitcode


if __name__ == '__main__':
    sys.exit(main())
