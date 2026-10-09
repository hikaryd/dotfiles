#!/usr/bin/env python3
"""Opt-in destination BBR for registered awg0 peers; never change global TCP."""

import argparse
import fcntl
import ipaddress
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

CONFIG = Path("/etc/awg-tcp-tuning.json")
HELPER = Path("/usr/local/libexec/awg-tcp-tuning.py")
UNIT_PATH = Path("/etc/systemd/system/awg-tcp-tuning.service")
SERVICE = "awg-tcp-tuning.service"
PARENT = "awg-quick@awg0.service"
INTERFACE = "awg0"
METRIC = 42705  # Reserved for durable routes; diagnostic trials use 42704.
UNIT = f"""[Unit]
Description=Opt-in TCP BBR for registered AWG peers
BindsTo={PARENT}
PartOf={PARENT}
After={PARENT}

[Service]
Type=oneshot
RemainAfterExit=yes
Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ExecStart=/usr/bin/python3 {HELPER} apply
ExecStop=/usr/bin/python3 {HELPER} cleanup

[Install]
WantedBy={PARENT}
"""


class TuningError(Exception):
    pass


def run(*args):
    """Keep command output internal: awg includes peer public keys."""
    try:
        result = subprocess.run(args, capture_output=True, text=True, check=False)
    except OSError:
        raise TuningError(f"Cannot execute {args[0]}") from None
    if result.returncode:
        raise TuningError(f"{args[0]} command failed (exit {result.returncode})")
    return result.stdout


def peer_cidr(value):
    if not isinstance(value, str):
        raise TuningError("Peers must be IPv4 /32 addresses")
    try:
        network = ipaddress.ip_network(value, strict=True)
    except (TypeError, ValueError):
        raise TuningError("Peers must be IPv4 /32 addresses") from None
    if network.version != 4 or network.prefixlen != 32:
        raise TuningError("Peers must be IPv4 /32 addresses")
    return str(network)


def read_peers():
    try:
        data = json.loads(CONFIG.read_text())
    except (OSError, ValueError):
        raise TuningError("Cannot read TCP tuning configuration") from None
    if not isinstance(data, dict) or set(data) != {"peers"} or not isinstance(data["peers"], list):
        raise TuningError("Configuration must contain only a peers list")
    return list(dict.fromkeys(peer_cidr(value) for value in data["peers"]))


def validate_registered(peers):
    if not peers:
        return
    allowed = set()
    for line in run("awg", "show", INTERFACE, "allowed-ips").splitlines():
        fields = line.split(maxsplit=1)
        if len(fields) == 2:
            allowed.update(value.strip() for value in fields[1].split(","))
    if any(peer not in allowed for peer in peers):
        raise TuningError("A requested address is not an actual awg0 peer AllowedIP")


def routes():
    try:
        data = json.loads(run("ip", "-j", "-4", "route", "show"))
    except ValueError:
        raise TuningError("Cannot decode awg0 routes") from None
    if not isinstance(data, list):
        raise TuningError("Unexpected awg0 route response")
    if not all(isinstance(route, dict) for route in data):
        raise TuningError("Unexpected awg0 route entry")
    return data


def destination(route):
    try:
        return peer_cidr(route.get("dst", ""))
    except TuningError:
        return None


def owned(route):
    metrics = route.get("metrics", {})
    if isinstance(metrics, list):
        metrics = {key: value for item in metrics for key, value in item.items()}
    return (
        destination(route) is not None
        and route.get("dev") == INTERFACE
        and route.get("protocol") == "static"
        and route.get("metric") == METRIC
        and metrics.get("congestion") == "bbr"
    )


def delete_route(peer):
    run("ip", "-4", "route", "del", peer, "dev", INTERFACE,
        "proto", "static", "metric", str(METRIC))


def cleanup():
    for route in routes():
        if owned(route):
            delete_route(destination(route))


def apply():
    peers = read_peers()
    validate_registered(peers)
    before = routes()
    # Never replace a route installed by AWG, an administrator, or another tool.
    for route in before:
        if destination(route) in peers and not owned(route):
            raise TuningError("A requested peer already has a route owned by another tool")
    existing = {destination(route) for route in before if owned(route)}
    added = []
    if peers:
        run("modprobe", "tcp_bbr")
    try:
        for peer in peers:
            if peer not in existing:
                run("ip", "-4", "route", "add", peer, "dev", INTERFACE,
                    "proto", "static", "metric", str(METRIC), "congctl", "lock", "bbr")
                added.append(peer)
        # A textual check is needed: iproute2 JSON omits the congctl lock flag.
        for peer in peers:
            output = run("ip", "-4", "route", "show", "exact", peer, "dev", INTERFACE)
            if "congctl lock bbr" not in output:
                raise TuningError("The requested BBR route was not installed with a lock")
    except TuningError:
        for peer in added:
            delete_route(peer)
        raise
    for route in before:
        if owned(route) and destination(route) not in peers:
            delete_route(destination(route))


def write_file(path, content, mode):
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    try:
        with os.fdopen(descriptor, "wb") as target:
            os.fchmod(target.fileno(), mode)
            target.write(content)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def save_peers(peers):
    write_file(CONFIG, (json.dumps({"peers": peers}) + "\n").encode(), 0o600)


def install(peers=None):
    # Do not let BindsTo start a currently inactive AWG service during installation.
    run("systemctl", "is-active", "--quiet", PARENT)
    if peers is not None:
        peers = list(dict.fromkeys(peer_cidr(peer) for peer in peers))
        validate_registered(peers)
    elif CONFIG.exists():
        validate_registered(read_peers())
    if Path(__file__).resolve() != HELPER.resolve():
        write_file(HELPER, Path(__file__).read_bytes(), 0o755)
    write_file(UNIT_PATH, UNIT.encode(), 0o644)
    if peers is not None:
        save_peers(peers)
    elif not CONFIG.exists():
        save_peers([])
    else:
        CONFIG.chmod(0o600)
    run("systemctl", "daemon-reload")
    run("systemctl", "enable", SERVICE)
    run("systemctl", "restart", SERVICE)


def disable():
    run("systemctl", "disable", "--now", SERVICE)
    cleanup()
    # A later full installer run must not restore a deliberately disabled opt-in.
    save_peers([])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("install", "apply", "cleanup", "disable"))
    parser.add_argument("--peer", action="append", help="install: explicitly opt in an IPv4 /32 peer")
    args = parser.parse_args()
    if args.peer is not None and args.action != "install":
        parser.error("--peer is only valid with install")
    if os.geteuid() != 0:
        parser.error("root is required")
    try:
        with open("/run/lock/awg-tcp-tuning.lock", "a") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            if args.action == "install":
                # ExecStop runs this helper too; release before waiting for systemd.
                fcntl.flock(lock, fcntl.LOCK_UN)
                install(args.peer)
            elif args.action == "disable":
                fcntl.flock(lock, fcntl.LOCK_UN)
                disable()
            elif args.action == "apply":
                apply()
            else:
                cleanup()
    except (TuningError, OSError) as error:
        print(f"TCP tuning failed: {error}", file=sys.stderr)
        return 1
    print(f"TCP tuning: {args.action} completed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
