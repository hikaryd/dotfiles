#!/usr/bin/env python3
"""Bounded VPS outbound benchmark. Isolated sing-box; never changes live selection.

Run on the VPS with permission to read its sing-box config. Temporary configs
contain credentials and are removed. Results describe each download endpoint,
not LAN/Wi-Fi/AWG capacity. Repeat --url to compare independent sources.
"""
import argparse
import copy
import json
import math
import secrets
import signal
import socket
import statistics
import subprocess
import sys
import tempfile
import time
from pathlib import Path
from urllib.parse import urlsplit

DEFAULT_URL = "https://speed.cloudflare.com/__down?bytes={bytes}"
STOP_TIMEOUT = 5
# Transport types from sing-box's outbound schema. Helpers such as selector,
# urltest, direct, block, dns and bridge are deliberately absent.
PROXY_TYPES = frozenset({"socks", "http", "shadowsocks", "vmess", "trojan", "naive",
                         "wireguard", "hysteria", "shadowtls", "vless", "tuic",
                         "hysteria2", "anytls", "snell", "tailcat", "tor", "ssh"})


def download_url(template, size):
    """Require an HTTP(S) endpoint; never accept embedded login credentials."""
    url = template.replace("{bytes}", str(size))
    try:
        parsed = urlsplit(url)
        valid = (parsed.scheme in ("http", "https") and parsed.hostname
                 and parsed.port != 0 and parsed.username is None
                 and parsed.password is None and not parsed.fragment
                 and not any(c.isspace() or ord(c) < 32 for c in url)
                 and "{" not in url and "}" not in url)
    except ValueError:
        valid = False
    if not valid:
        raise ValueError("URLs must be HTTP(S), without userinfo, fragments or unknown placeholders")
    return url


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def failed_trial(error, **values):
    return {"ok": False, "http": 0, "bytes": 0, "mbps": None,
            "ttfb_ms": None, "seconds": None, "error": error, **values}


def measurement(port, password, size, timeout, url=DEFAULT_URL, deadline=None):
    remaining = timeout + 3 if deadline is None else deadline - time.monotonic()
    if remaining <= 0:
        return failed_trial("duration_budget_exhausted", skipped=True)
    curl_timeout = min(timeout, remaining)
    try:
        # stdin keeps the random SOCKS password out of the process command line.
        # --disable must be first: a user's .curlrc must not change the probe.
        result = subprocess.run([
            "curl", "--disable", "--config", "-", "--globoff", "--noproxy", "",
            "--proxy", f"socks5h://127.0.0.1:{port}", "--silent",
            "--output", "/dev/null", "--connect-timeout", str(min(5, curl_timeout)),
            "--max-time", str(curl_timeout), "--write-out", "%{json}",
            "--url", download_url(url, size),
        ], input=f'proxy-user = "bench:{password}"\n', capture_output=True,
            text=True, check=False, timeout=min(timeout + 3, remaining))
        data = json.loads(result.stdout)
        http, downloaded = data["http_code"], data["size_download"]
        speed, ttfb, seconds = (data["speed_download"],
                                data["time_starttransfer"], data["time_total"])
        if any(not isinstance(v, (int, float)) or isinstance(v, bool)
               or not math.isfinite(v) or v < 0
               for v in (http, downloaded, speed, ttfb, seconds)) or seconds <= 0:
            return failed_trial("invalid_curl_metrics")
        error = ("curl_failed" if result.returncode else
                 "unexpected_http" if http != 200 else
                 "incomplete_download" if downloaded != size else None)
        valid = error is None
        return {"ok": valid, "http": http, "bytes": downloaded,
                "mbps": round(speed * 8 / 1e6, 2) if valid else None,
                "ttfb_ms": round(ttfb * 1000, 1), "seconds": round(seconds, 3),
                "curl_exit": result.returncode, "error": error}
    except subprocess.TimeoutExpired:
        error = ("duration_budget_exhausted" if deadline is not None
                 and time.monotonic() >= deadline else "curl_timeout")
        return failed_trial(error)
    except OSError:
        return failed_trial("curl_unavailable")
    except (ValueError, KeyError, TypeError):
        return failed_trial("invalid_curl_metrics")


def stop_process(proc):
    if proc.poll() is None:
        proc.terminate()
    try:
        proc.wait(timeout=STOP_TIMEOUT)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait(timeout=STOP_TIMEOUT)


def benchmark(node, samples, size, timeout, url=DEFAULT_URL, deadline=None):
    outbound = copy.deepcopy(node)
    outbound["tag"] = "bench"
    port, password = free_port(), secrets.token_hex(16)
    config = {
        "log": {"disabled": True},
        "dns": {"servers": [{"type": "local", "tag": "local"}]},
        "inbounds": [{"type": "mixed", "listen": "127.0.0.1", "listen_port": port,
                      "users": [{"username": "bench", "password": password}]}],
        "outbounds": [outbound],
        "route": {"final": "bench", "auto_detect_interface": True,
                  "default_domain_resolver": {"server": "local"}},
    }
    with tempfile.TemporaryDirectory(prefix="vpn-bench-") as tmp:
        path = Path(tmp) / "config.json"
        path.touch(mode=0o600)
        path.write_text(json.dumps(config), encoding="utf-8")
        proc = subprocess.Popen(["sing-box", "run", "-c", str(path)],
                                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            for _ in range(50):
                if deadline is not None and time.monotonic() >= deadline:
                    raise RuntimeError("duration budget exhausted")
                if proc.poll() is not None:
                    raise RuntimeError("isolated sing-box exited")
                try:
                    with socket.create_connection(("127.0.0.1", port), timeout=0.1):
                        break
                except OSError:
                    time.sleep(0.1)
            else:
                raise RuntimeError("isolated listener did not start")
            return [measurement(port, password, size, timeout, url, deadline)
                    for _ in range(samples)]
        finally:
            stop_process(proc)


def select_candidates(config, only, extra, all_nodes=False):
    try:
        outbounds = config["outbounds"]
        nodes = {o["tag"]: o for o in outbounds}
        if len(nodes) != len(outbounds):
            raise ValueError("outbound tags must be unique")
        if all_nodes:
            tags = [tag for tag, node in nodes.items() if node.get("type") in PROXY_TYPES]
        else:
            tags = list(dict.fromkeys(only or nodes["auto"]["outbounds"] + extra))
        if not tags or (not all_nodes and len(tags) > 16) or any(
                tag not in nodes or nodes[tag]["type"] not in PROXY_TYPES or tag == "VPS-direct"
                for tag in tags):
            raise ValueError("choose existing proxy tags; maximum 16 unless --all; VPS-direct is reserved")
        return [{"type": "direct", "tag": "VPS-direct"}] + [nodes[tag] for tag in tags]
    except (KeyError, TypeError, AttributeError):
        raise ValueError("invalid sing-box outbounds or missing auto pool") from None


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", default="/etc/sing-box/config.json")
    parser.add_argument("--samples", type=int, help="Default 2; --all screening default 1")
    parser.add_argument("--bytes", type=int, help="Default 10000000; --all screening default 1000000")
    parser.add_argument("--timeout", type=int, help="Per download seconds; default 20, --all default 10")
    parser.add_argument("--extra", action="append", default=[], help="Exact manual outbound tag")
    selection = parser.add_mutually_exclusive_group()
    selection.add_argument("--only", action="append", default=[], help="Test only these exact proxy tags")
    selection.add_argument("--all", action="store_true", help=(
        "Screen every real proxy outbound, including all supported protocols. "
        "Does not read the subscription or change live routing."))
    parser.add_argument("--max-duration", type=float, metavar="SECONDS", help=(
        "Overall probe budget, 0..86400 seconds (positive). Default is computed from "
        "all planned trial timeouts, capped at 86400. Exhausted trials are reported as skipped; "
        "process cleanup may take up to 10 additional seconds."))
    parser.add_argument("--url", action="append", metavar="URL", help=(
        "Repeat for 1..4 sources; defaults to Cloudflare. Optional {bytes} placeholder. "
        "Static downloads must match --bytes exactly. No embedded login credentials."))
    args = parser.parse_args(argv)
    args.samples = args.samples if args.samples is not None else (1 if args.all else 2)
    args.bytes = args.bytes if args.bytes is not None else (1_000_000 if args.all else 10_000_000)
    args.timeout = args.timeout if args.timeout is not None else (10 if args.all else 20)
    if not (1 <= args.samples <= 5 and 1 <= args.bytes <= 50_000_000 and 1 <= args.timeout <= 30):
        parser.error("samples 1..5, bytes 1..50000000, timeout 1..30")
    if args.max_duration is not None and (not math.isfinite(args.max_duration)
                                         or not 0 < args.max_duration <= 86400):
        parser.error("max-duration must be positive and at most 86400 seconds")
    templates = args.url or [DEFAULT_URL]
    if not 1 <= len(templates) <= 4:
        parser.error("choose 1..4 download sources")
    try:
        urls = list(dict.fromkeys(download_url(url, args.bytes) for url in templates))
        config = json.loads(Path(args.config).read_text(encoding="utf-8"))
        candidates = select_candidates(config, args.only, args.extra, args.all)
    except OSError:
        parser.error("cannot read sing-box config")
    except ValueError as exc:
        # JSONDecodeError includes positions but no credentials; other messages
        # above are fixed text and never include a supplied URL/config value.
        parser.error(str(exc) if not isinstance(exc, json.JSONDecodeError)
                     else "invalid sing-box config JSON")
    sources = [(f"{i + 1}:{urlsplit(url).hostname}", url) for i, url in enumerate(urls)]
    results = {(source, node["tag"]): [] for source, _ in sources for node in candidates}
    protocols = {node["tag"]: node["type"] for node in candidates}
    total = len(candidates) * len(sources) * args.samples
    worst_case = total * (args.timeout + 3 + 10 + 2 * STOP_TIMEOUT)
    duration = args.max_duration if args.max_duration is not None else min(worst_case, 86400)
    started, completed, failure_count, skipped_count = time.monotonic(), 0, 0, 0
    deadline = started + duration
    print(json.dumps({"event": "plan", "proxy_nodes": len(candidates) - 1,
                      "protocols": sorted(set(protocols.values()) - {"direct"}),
                      "sources": [source for source, _ in sources], "total_trials": total,
                      "samples": args.samples, "bytes": args.bytes, "timeout": args.timeout,
                      "planned_download_bytes": total * args.bytes,
                      "duration_budget_seconds": duration, "worst_case_seconds": worst_case}), flush=True)
    # Rotate both sources and nodes so temporary load is not tied to a candidate.
    for round_no in range(args.samples):
        source_shift, node_shift = round_no % len(sources), round_no % len(candidates)
        for source, url in sources[source_shift:] + sources[:source_shift]:
            for node in candidates[node_shift:] + candidates[:node_shift]:
                trial_started = time.monotonic()
                try:
                    if trial_started >= deadline:
                        trial = failed_trial("duration_budget_exhausted", skipped=True)
                    else:
                        trial = benchmark(node, 1, args.bytes, args.timeout, url, deadline)[0]
                except RuntimeError as exc:
                    errors = {"isolated sing-box exited": "isolated_process_exited",
                              "isolated listener did not start": "isolated_start_timeout",
                              "duration budget exhausted": "duration_budget_exhausted"}
                    trial = failed_trial(errors.get(str(exc), "isolated_process_failed"))
                except (OSError, subprocess.TimeoutExpired):
                    trial = failed_trial("isolated_process_failed")
                trial["wall_seconds"] = round(time.monotonic() - trial_started, 3)
                results[source, node["tag"]].append(trial)
                completed += 1
                skipped_count += bool(trial.get("skipped"))
                failure_count += not trial["ok"] and not trial.get("skipped", False)
                now = time.monotonic()
                print(json.dumps({"event": "trial", "round": round_no + 1, "source": source,
                                  "node": node["tag"], "protocol": node["type"], "trial": trial,
                                  "progress": {"completed": completed, "total": total,
                                               "failures": failure_count, "skipped": skipped_count,
                                               "elapsed_seconds": round(now - started, 3),
                                               "remaining_budget_seconds": round(max(0, deadline - now), 3)}},
                                 ensure_ascii=False), flush=True)
    failed = False
    for (source, tag), trials in results.items():
        speeds = [t["mbps"] for t in trials if t["ok"]]
        failed |= not speeds
        skipped = sum(bool(t.get("skipped")) for t in trials)
        print(json.dumps({"event": "summary", "source": source, "node": tag,
                          "protocol": protocols[tag], "trials": trials, "successful": len(speeds),
                          "failures": len(trials) - len(speeds) - skipped, "skipped": skipped,
                          "median_mbps": statistics.median(speeds) if speeds else None},
                         ensure_ascii=False), flush=True)
    print(json.dumps({"event": "complete", "completed": completed, "attempted": completed - skipped_count,
                      "failures": failure_count, "skipped": skipped_count,
                      "elapsed_seconds": round(time.monotonic() - started, 3)}), flush=True)
    return 1 if failed or skipped_count else 0


def cancel(_signum, _frame):
    raise KeyboardInterrupt


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, cancel)
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        print("Benchmark cancelled; isolated process and temporary config removed.", file=sys.stderr)
        sys.exit(130)
