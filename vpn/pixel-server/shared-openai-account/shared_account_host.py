"""Host fan-out controller. All external output is metadata-only."""

from __future__ import annotations

import fcntl
import json
import os
import stat
import subprocess
import sys
import tempfile
import time
from pathlib import Path

from hermes_account import AccountError, build_cli_parser, identity, validate_policy

BASE = "/opt/hermes-shared-account"
TARGETS = (
    (
        "summary",
        "/etc/fintech-summary-bot/hermes-codex/auth.json",
        10001,
        10001,
    ),
    (
        "summary-legacy",
        "/opt/fintech-summary-bot-shared/secrets/codex/auth.json",
        10001,
        10001,
    ),
    (
        "open-design",
        "/var/lib/docker/volumes/open-design_open_design_data/_data/codex/auth.json",
        1001,
        1001,
    ),
    ("oif", "/etc/oif-bot/hermes-codex/auth.json", 999, 999),
)
ACK = "/root/.hermes/shared-account-status.json"


def validate_document(document):
    policy = validate_policy(document.get("hermes_account"))
    tokens = document["tokens"]
    who = identity(tokens["access_token"], minimum_ttl=90)
    if (
        document.get("auth_mode") != "chatgptAuthTokens"
        or document.get("OPENAI_API_KEY") is not None
        or tokens.get("refresh_token") != ""
        or tokens.get("id_token") != tokens["access_token"]
        or tokens.get("account_id") != policy["account_id"]
        or any(who[key] != policy[key] for key in ("account_id", "email"))
    ):
        raise AccountError("invalid_projection")
    return policy


def atomic_write(path, document, uid, gid):
    path = Path(path)
    if path.is_symlink():
        raise ValueError("symlink target denied")
    if path.exists():
        info = path.stat()
        if not stat.S_ISREG(info.st_mode):
            raise ValueError("nonregular target denied")
        try:
            old = json.loads(path.read_text())
        except (ValueError, TypeError):
            old = {}
        if not isinstance(old, dict):
            old = {}
        if (
            old.get("auth_mode") == document["auth_mode"]
            and old.get("tokens") == document["tokens"]
            and old.get("hermes_account") == document["hermes_account"]
            and old.get("OPENAI_API_KEY") is None
            and stat.S_IMODE(info.st_mode) == 0o600
            and (info.st_uid, info.st_gid) == (uid, gid)
        ):
            return False
    fd, temporary = tempfile.mkstemp(prefix=".auth-", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as handle:
            os.fchmod(handle.fileno(), 0o600)
            os.fchown(handle.fileno(), uid, gid)
            json.dump(document, handle)
            handle.write("\n")
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary, path)
        directory = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)
    return True


def incus(script, command, target=None, *, document=None, interactive=False):
    argv = [
        "incus",
        "exec",
        "hermes-agent",
        "--",
        "/usr/local/lib/hermes-agent/venv/bin/python",
        BASE + "/" + script,
        command,
    ]
    if target is not None:
        argv.append(target)
    if interactive:
        result = subprocess.run(argv, check=False)
        if result.returncode:
            raise AccountError("owner_login_failed")
        return None
    result = subprocess.run(
        argv,
        input=json.dumps(document) if document is not None else None,
        capture_output=True,
        text=True,
        timeout=90,
        check=False,
    )
    if result.returncode:
        # Do not echo stderr: a native library error can contain OAuth response material.
        raise AccountError("owner_operation_failed_" + command.lstrip("_"))
    return json.loads(result.stdout)


def owner(command, target=None):
    return incus("hermes_account.py", command, target)


def target_status(path, policy, uid=0, gid=0):
    try:
        actual = validate_document(json.loads(Path(path).read_text()))
        info = Path(path).lstat()
        return (
            actual == policy
            and stat.S_ISREG(info.st_mode)
            and stat.S_IMODE(info.st_mode) == 0o600
            and (info.st_uid, info.st_gid) == (uid, gid)
        )
    except (OSError, ValueError, KeyError, TypeError, AccountError):
        return False


def status():
    owner_status = owner("status")
    selected = owner_status["selected"]
    targets = {
        name: target_status(path, selected, uid, gid)
        for name, path, uid, gid in TARGETS
    }
    remote = incus("shared_account_host.py", "_status")
    targets["hermes-cli"] = remote.get("selected") == selected
    targets["hermes-owner"] = owner_status.get("owner_ready") is True
    return {
        "selected": selected,
        "targets": targets,
        "fleet_status": "converged" if all(targets.values()) else "pending",
    }


def sync():
    document = owner("_snapshot")
    policy = validate_document(document)
    errors = []
    for name, path, uid, gid in TARGETS:
        try:
            atomic_write(path, document, uid, gid)
        except (OSError, ValueError, TypeError):
            errors.append(name)
    try:
        incus("shared_account_host.py", "_publish", document=document)
    except AccountError:
        errors.append("hermes-cli")
    result = status()
    result["errors"] = errors
    if errors or result["selected"] != policy:
        result["fleet_status"] = "pending"
    incus("shared_account_host.py", "_ack", document=result)
    return result


def internal(command):
    if command == "_publish":
        document = json.load(sys.stdin)
        policy = validate_document(document)
        atomic_write("/root/.codex/auth.json", document, 0, 0)
        return {"selected": policy}
    if command == "_status":
        document = json.loads(Path("/root/.codex/auth.json").read_text())
        policy = validate_document(document)
        return {
            "selected": policy
            if target_status("/root/.codex/auth.json", policy)
            else None
        }
    if command == "_ack":
        result = json.load(sys.stdin)
        safe = {
            "selected": validate_policy(result["selected"]),
            "fleet_status": "converged"
            if result["fleet_status"] == "converged"
            else "pending",
            "checked_at": time.time(),
            "targets": {
                name: result.get("targets", {}).get(name) is True
                for name in (
                    "summary",
                    "summary-legacy",
                    "open-design",
                    "oif",
                    "hermes-cli",
                    "hermes-owner",
                )
            },
        }
        path = Path(ACK)
        fd, temporary = tempfile.mkstemp(prefix=".account-status-", dir=path.parent)
        try:
            with os.fdopen(fd, "w") as handle:
                os.fchmod(handle.fileno(), 0o600)
                json.dump(safe, handle)
                handle.flush()
                os.fsync(handle.fileno())
            os.replace(temporary, path)
        finally:
            if os.path.exists(temporary):
                os.unlink(temporary)
        return {"recorded": True}
    raise AccountError("unknown_internal_command")


def main():
    parser = build_cli_parser(host=True)
    args = parser.parse_args()
    if args.command is None:
        parser.print_help()
        raise SystemExit(0)
    if os.geteuid() != 0:
        parser.error("root-only command")
    try:
        if args.command.startswith("_"):
            result = internal(args.command)
        elif args.command == "login":
            # Interactive flow must not hold the fan-out lock for minutes.
            incus("hermes_account.py", "login", interactive=True)
            with open("/run/lock/hermes-shared-account.lock", "a") as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                result = sync()
        else:
            with open("/run/lock/hermes-shared-account.lock", "a") as lock:
                fcntl.flock(lock, fcntl.LOCK_EX)
                if args.command in ("use", "initialize"):
                    owner(args.command, args.target)
                    result = sync()
                elif args.command == "sync":
                    result = sync()
                elif args.command == "status":
                    result = status()
                else:
                    result = owner(args.command, args.target)
        print(json.dumps(result, ensure_ascii=False))
        if result.get("fleet_status") == "pending":
            raise SystemExit(2)
    except Exception as exc:  # noqa: BLE001 - credential redaction boundary
        code = str(exc) if isinstance(exc, AccountError) else type(exc).__name__
        print(json.dumps({"error": code}), file=sys.stderr)
        raise SystemExit(1)


if __name__ == "__main__":
    main()
