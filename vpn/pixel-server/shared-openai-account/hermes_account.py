"""Hermes-local account owner. CLI emits identities, never credentials."""

from __future__ import annotations

import argparse
import base64
import copy
import hashlib
import json
import math
import os
import sys
import time
import urllib.request
import uuid
from datetime import datetime, timezone
from pathlib import Path

POLICY = "shared_account"
ARCHIVE = "shared_account_archive"
PROVIDER = "openai-codex"


class AccountError(RuntimeError):
    pass


def identity(token, *, minimum_ttl=0):
    try:
        if not isinstance(token, str) or len(token) > 65536:
            raise ValueError
        parts = token.split(".")
        if len(parts) != 3:
            raise ValueError
        claims = json.loads(
            base64.urlsafe_b64decode(parts[1] + "=" * (-len(parts[1]) % 4))
        )
        account = claims["https://api.openai.com/auth"]["chatgpt_account_id"]
        email = claims["https://api.openai.com/profile"]["email"]
        expiry = claims["exp"]
        if (
            not isinstance(account, str)
            or not account.strip()
            or not isinstance(email, str)
            or "@" not in email
            or any(ord(c) < 32 for c in account + email)
            or isinstance(expiry, bool)
            or not isinstance(expiry, (int, float))
            or not math.isfinite(expiry)
            or (minimum_ttl and expiry <= time.time() + minimum_ttl)
        ):
            raise ValueError
        return {"account_id": account, "email": email, "expires_at": expiry}
    except (ValueError, KeyError, TypeError, AttributeError):
        raise AccountError("invalid_or_expiring_credential") from None


def validate_policy(value):
    try:
        if (
            not isinstance(value, dict)
            or type(value.get("schema_version")) is not int
            or value["schema_version"] != 1
        ):
            raise ValueError
        uuid.UUID(value["generation"])
        for key in ("account_id", "email"):
            if (
                not isinstance(value[key], str)
                or not value[key].strip()
                or any(ord(c) < 32 for c in value[key])
            ):
                raise ValueError
        return {
            key: value[key]
            for key in ("schema_version", "generation", "account_id", "email")
        }
    except (ValueError, KeyError, TypeError, AttributeError):
        raise AccountError("invalid_shared_account_policy") from None


def registry(store):
    """Live grants outrank archives; conflicting live revisions fail closed."""
    result = {}
    candidates = []
    provider = store.get("providers", {}).get(PROVIDER, {})
    candidates.append(
        {
            "tokens": provider.get("tokens", {}),
            "last_refresh": provider.get("last_refresh"),
        }
    )
    candidates.extend(store.get("credential_pool", {}).get(PROVIDER, []))
    live_count = len(candidates)
    candidates.extend(store.get(ARCHIVE, {}).values())
    for index, candidate in enumerate(candidates):
        if not isinstance(candidate, dict):
            raise AccountError("malformed_credential_registry")
        tokens = candidate.get("tokens", candidate)
        try:
            who = identity(tokens.get("access_token"))
        except AccountError:
            continue
        if (
            not isinstance(tokens.get("refresh_token"), str)
            or not tokens["refresh_token"]
        ):
            continue
        old = result.get(who["account_id"])
        if old is not None:
            if index < live_count and any(
                old["tokens"][key] != tokens[key]
                for key in ("access_token", "refresh_token")
            ):
                raise AccountError("conflicting_live_grants_reconcile_natively")
            continue
        result[who["account_id"]] = {
            "tokens": {key: tokens[key] for key in ("access_token", "refresh_token")},
            "last_refresh": candidate.get("last_refresh"),
        }
    return result


def resolve_target(records, target):
    matches = [
        record
        for account, record in records.items()
        if target in (account, identity(record["tokens"]["access_token"])["email"])
    ]
    if len(matches) != 1:
        raise AccountError("account_missing_or_ambiguous_use_exact_account_id")
    return matches[0]


def activate(store, record, *, generation=None, minimum_ttl=600):
    """Pure atomic-store transformation: archive old grant, promote exactly one."""
    result = copy.deepcopy(store)
    records = registry(store)
    who = identity(record["tokens"]["access_token"], minimum_ttl=minimum_ttl)
    previous = store.get(POLICY)
    if generation is None:
        if previous and validate_policy(previous)["account_id"] == who["account_id"]:
            generation = previous["generation"]
        else:
            generation = str(uuid.uuid4())
    policy = validate_policy(
        {
            "schema_version": 1,
            "generation": generation,
            "account_id": who["account_id"],
            "email": who["email"],
        }
    )
    records.pop(who["account_id"], None)
    result[ARCHIVE] = records
    result[POLICY] = policy
    old_provider = result.setdefault("providers", {}).setdefault(PROVIDER, {})
    old_provider.update(
        tokens=copy.deepcopy(record["tokens"]),
        last_refresh=record.get("last_refresh"),
        auth_mode="chatgpt",
        account_id=who["account_id"],
        email=who["email"],
    )
    # One canonical native device_code row: Hermes syncs its refresh back to singleton.
    current_rows = store.get("credential_pool", {}).get(PROVIDER, [])
    row_id = next(
        (
            r["id"]
            for r in current_rows
            if r.get("source") == "device_code"
            and r.get("access_token") == record["tokens"]["access_token"]
        ),
        uuid.uuid4().hex[:12],
    )
    result.setdefault("credential_pool", {})[PROVIDER] = [
        {
            "id": row_id,
            "label": "Shared account: " + who["email"],
            "auth_type": "oauth",
            "source": "device_code",
            "priority": 0,
            **record["tokens"],
            "last_refresh": record.get("last_refresh"),
        }
    ]
    result["active_provider"] = PROVIDER
    return result


def validate_owner_store(store):
    policy = validate_policy(store.get(POLICY))
    tokens = store.get("providers", {}).get(PROVIDER, {}).get("tokens", {})
    who = identity(tokens.get("access_token"))
    rows = store.get("credential_pool", {}).get(PROVIDER, [])
    if (
        store.get("active_provider") != PROVIDER
        or any(who[key] != policy[key] for key in ("account_id", "email"))
        or len(rows) != 1
        or rows[0].get("source") != "device_code"
        or any(
            rows[0].get(key) != tokens.get(key)
            for key in ("access_token", "refresh_token")
        )
        or not tokens.get("refresh_token")
    ):
        raise AccountError("owner_single_account_invariant_failed")
    return policy


def snapshot(store):
    policy = validate_owner_store(store)
    tokens = store["providers"][PROVIDER]["tokens"]
    who = identity(tokens["access_token"], minimum_ttl=600)
    if any(who[key] != policy[key] for key in ("account_id", "email")):
        raise AccountError("owner_policy_token_mismatch")
    token = tokens["access_token"]
    return {
        "auth_mode": "chatgptAuthTokens",
        "OPENAI_API_KEY": None,
        "tokens": {
            "id_token": token,
            "access_token": token,
            "refresh_token": "",
            "account_id": policy["account_id"],
        },
        "last_refresh": store["providers"][PROVIDER].get("last_refresh")
        or datetime.now(timezone.utc).isoformat(),
        "hermes_account": policy,
    }


def probe(token):
    who = identity(token, minimum_ttl=600)
    request = urllib.request.Request(
        "https://chatgpt.com/backend-api/wham/usage",
        headers={
            "Authorization": "Bearer " + token,
            "ChatGPT-Account-ID": who["account_id"],
            "User-Agent": "codex_cli_rs/0.145.0",
        },
    )

    # No ambient proxies, redirects or third-party endpoints for the credential probe.
    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, req, fp, code, msg, headers, newurl):
            return None

    try:
        with urllib.request.build_opener(
            urllib.request.ProxyHandler({}), NoRedirect()
        ).open(request, timeout=20) as response:
            data = json.load(response)
            if (
                response.status != 200
                or data.get("rate_limit", {}).get("allowed") is False
            ):
                raise AccountError("account_unavailable_or_quota_exhausted")
    except (OSError, ValueError, AccountError):
        raise AccountError("account_probe_failed_or_quota_exhausted") from None


def native_api():
    sys.path.insert(0, "/usr/local/lib/hermes-agent")
    from hermes_cli.auth import (
        _auth_store_lock,
        _load_auth_store,
        _save_auth_store,
        resolve_codex_runtime_credentials,
    )

    return (
        _auth_store_lock,
        _load_auth_store,
        _save_auth_store,
        resolve_codex_runtime_credentials,
    )


def native_refresh_pair(record):
    from hermes_cli.auth import refresh_codex_oauth_pure

    tokens = record["tokens"]
    fresh = refresh_codex_oauth_pure(
        tokens["access_token"], tokens["refresh_token"], timeout_seconds=20
    )
    return {
        "tokens": {key: fresh[key] for key in ("access_token", "refresh_token")},
        "last_refresh": fresh["last_refresh"],
    }


def persist_renewed(store, old_record, fresh_record):
    """Save a rotated pair BEFORE quota probe, without selecting an archived account."""
    result = copy.deepcopy(store)
    account = identity(old_record["tokens"]["access_token"])["account_id"]
    current = result.get("providers", {}).get(PROVIDER, {}).get("tokens", {})
    if current.get("refresh_token") == old_record["tokens"]["refresh_token"]:
        result["providers"][PROVIDER].update(copy.deepcopy(fresh_record))
    for row in result.get("credential_pool", {}).get(PROVIDER, []):
        if row.get("refresh_token") == old_record["tokens"]["refresh_token"]:
            row.update(copy.deepcopy(fresh_record["tokens"]))
            row["last_refresh"] = fresh_record.get("last_refresh")
    if account in result.get(ARCHIVE, {}):
        result[ARCHIVE][account] = copy.deepcopy(fresh_record)
    return result


def native_login():
    from hermes_cli.auth import _codex_device_code_login

    return _codex_device_code_login()


def run(action, target=None):
    lock, load, save, refresh = native_api()
    if action in ("list", "status"):
        with lock():
            store = load()
            selected = validate_policy(store[POLICY]) if POLICY in store else None
            if action == "status":
                result = {
                    "selected": selected,
                    "initialized": POLICY in store,
                    "fleet_status": "pending",
                    "owner_ready": False,
                }
                try:
                    validate_owner_store(store)
                    identity(
                        store["providers"][PROVIDER]["tokens"]["access_token"],
                        minimum_ttl=90,
                    )
                    result["owner_ready"] = True
                    ack = json.loads(
                        Path("/root/.hermes/shared-account-status.json").read_text()
                    )
                    if (
                        validate_policy(ack.get("selected")) == selected
                        and ack.get("fleet_status") == "converged"
                        and time.time() - float(ack["checked_at"]) < 180
                    ):
                        result["fleet_status"] = "converged"
                except (OSError, ValueError, TypeError, KeyError, AccountError):
                    pass
                return result
            return {
                "accounts": [
                    identity(r["tokens"]["access_token"])
                    for r in registry(store).values()
                ],
                "selected": selected,
            }
    if action == "login":
        record = (
            native_login()
        )  # Pure device flow; user interaction outside owner lock.
        who = identity(record["tokens"]["access_token"], minimum_ttl=600)
        with lock():
            store = load()
            selected = validate_policy(store[POLICY])
            if who["account_id"] == selected["account_id"]:
                save(activate(store, record))
            else:
                store.setdefault(ARCHIVE, {})[who["account_id"]] = {
                    "tokens": record["tokens"],
                    "last_refresh": record.get("last_refresh"),
                }
                save(store)
            return {"added": who, "selected": selected, "selection_changed": False}
    if action in ("use", "check", "initialize"):
        with lock():
            store = load()
            if action == "initialize" and POLICY in store:
                return {"selected": validate_policy(store[POLICY]), "changed": False}
            records = registry(store)
            if action == "initialize":
                target = identity(
                    store["providers"][PROVIDER]["tokens"]["access_token"]
                )["account_id"]
            record = resolve_target(records, target)
            previous_identity = identity(record["tokens"]["access_token"])
            if previous_identity["expires_at"] < time.time() + 3600:
                fresh = native_refresh_pair(record)
                store = persist_renewed(store, record, fresh)
                save(store)  # A rejected quota probe must not discard a rotated grant.
                record = fresh
                if (
                    identity(record["tokens"]["access_token"])["account_id"]
                    != previous_identity["account_id"]
                ):
                    raise AccountError("refreshed_account_mismatch")
            probe(record["tokens"]["access_token"])
            next_store = activate(store, record)
            if action != "check":
                save(next_store)
            return {
                "selected": next_store[POLICY],
                "checked": True,
                "changed": action != "check",
                "fleet_status": "pending" if action != "check" else "not_changed",
            }
    if action == "_snapshot":
        # Native per-process flock serializes grant rotation. Never invoke CLI refresh.
        with lock():
            store = load()
            policy = validate_policy(store.get(POLICY))
            try:
                validate_owner_store(store)
            except AccountError:
                record = resolve_target(registry(store), policy["account_id"])
                # Reconcile a native login without changing the selected policy.
                aligned = activate(
                    store, record, generation=policy["generation"], minimum_ttl=0
                )
                save(aligned)
        refresh(refresh_skew_seconds=3600)
        with lock():
            return snapshot(load())
    raise AccountError("unknown_command")


def verify_native_guard_manifest(
    manifest_path=Path("/opt/hermes-shared-account/DEPLOYED"),
):
    """Refuse new account selection after an unverified native Hermes upgrade."""
    try:
        manifest = json.loads(manifest_path.read_text())
        base = Path("/usr/local/lib/hermes-agent")
        for relative in (
            "hermes_cli/auth.py",
            "agent/credential_pool.py",
            "hermes_cli/shared_account_guard.py",
        ):
            expected = manifest["native_sha256"][relative]
            if (
                not isinstance(expected, str)
                or hashlib.sha256((base / relative).read_bytes()).hexdigest()
                != expected
            ):
                raise ValueError
    except (OSError, ValueError, KeyError, TypeError):
        raise AccountError("native_guard_changed_revalidate_upgrade") from None


class HelpFormatter(argparse.RawDescriptionHelpFormatter):
    def add_usage(self, usage, actions, groups, prefix=None):
        super().add_usage(
            usage, actions, groups, "Использование: " if prefix is None else prefix
        )


def build_cli_parser(*, host=False, argv=None):
    arguments = sys.argv[1:] if argv is None else argv
    internal_commands = (
        ("initialize", "_publish", "_status", "_ack")
        if host
        else ("initialize", "_snapshot")
    )
    if arguments and arguments[0] in internal_commands:
        # Keep the existing machine protocol; never advertise it in public help.
        parser = argparse.ArgumentParser(prog="hermes-account")
        parser.add_argument("command", choices=internal_commands)
        parser.add_argument("target", nargs="?")
        return parser

    command = "haccount" if host else "hermes-account"
    delivery = (
        "На сервере use сразу доставляет авторизацию всем сервисам."
        if host
        else "Из Hermes изменения доходят до сервисов примерно за минуту."
    )
    parser = argparse.ArgumentParser(
        prog="hermes-account",
        description="Единый OpenAI-аккаунт для Hermes, OIF, Summary, open-design и crypto-card.",
        formatter_class=HelpFormatter,
        add_help=False,
        epilog=(
            f"Примеры:\n"
            f"  {command} list                     Сохранённые аккаунты\n"
            f"  {command} login                    Добавить аккаунт через браузер\n"
            f"  {command} use user@example.com     Переключить общий аккаунт\n"
            f"  {command} status                   Проверить результат\n"
            f"  {command} use --help               Подробнее о команде\n\n"
            "Для уже добавленного аккаунта достаточно use; login повторять не нужно.\n"
            f"{delivery}\n"
            "converged — авторизация доставлена; pending — доставка ещё не завершена.\n"
            "Сервисы не перезапускаются, текущие запросы не отменяются.\n"
            "Статус авторизации не заменяет проверку работоспособности сервисов.\n"
            + (
                "\nНа Маке haccount — ваш SSH-alias для серверной команды hermes-account.\n"
                if host
                else ""
            )
        ),
    )
    parser._optionals.title = "Параметры"
    parser.add_argument("-h", "--help", action="help", help="показать справку и выйти")
    commands = {
        "list": (
            "список сохранённых аккаунтов",
            "Показывает email и account ID сохранённых аккаунтов, без токенов.",
        ),
        "login": (
            "добавить аккаунт через вход в браузере",
            "Показывает ссылку и код для входа. Новый аккаунт сохраняется, но login не переключает на него сервисы.\nПосле входа выполните use. Повторный вход в текущий аккаунт обновляет его авторизацию.",
        ),
        "use": (
            "переключить общий аккаунт",
            f"Выбирает аккаунт по email или точному account ID из list.\nПри неоднозначном email используйте ID. {delivery}\nТекущие запросы не отменяются; следующие используют новую авторизацию.",
        ),
        "check": (
            "проверить аккаунт без переключения",
            "Проверяет авторизацию и доступную квоту. При необходимости обновляет и сохраняет токен,\nно не меняет выбранный аккаунт.",
        ),
        "status": (
            "текущий аккаунт и состояние доставки",
            "converged — авторизация доставлена; pending — доставка ещё не завершена.\nПоказывает состояние авторизации, а не здоровье всех бизнес-функций.",
        ),
    }
    if host:
        commands["sync"] = (
            "повторить доставку текущей авторизации",
            "Повторно доставляет текущую авторизацию сервисам. Аккаунт не меняет.\nИспользуйте при pending; таймер также повторяет доставку автоматически.",
        )
    subparsers = parser.add_subparsers(
        dest="command", title="Команды", metavar="КОМАНДА"
    )
    for name, (summary, detail) in commands.items():
        example_target = " user@example.com" if name in ("use", "check") else ""
        subparser = subparsers.add_parser(
            name,
            help=summary,
            description=detail,
            add_help=False,
            formatter_class=HelpFormatter,
            epilog=f"Пример:\n  {command} {name}{example_target}",
        )
        subparser._optionals.title = "Параметры"
        subparser._positionals.title = "Аргументы"
        subparser.add_argument(
            "-h", "--help", action="help", help="показать справку и выйти"
        )
        if name in ("use", "check"):
            subparser.add_argument(
                "target",
                metavar="EMAIL_ИЛИ_ID",
                help="email или точный account ID из команды list",
            )
        else:
            subparser.set_defaults(target=None)
    return parser


def main():
    parser = build_cli_parser()
    args = parser.parse_args()
    if args.command is None:
        parser.print_help()
        raise SystemExit(0)
    if os.geteuid() != 0:
        parser.error("owner-only command")
    if (
        args.command in ("use", "login")
        and not Path("/opt/hermes-shared-account/DEPLOYED").is_file()
    ):
        parser.error("rollout_not_complete_account_switch_disabled")
    if args.command == "_snapshot" and sys.stdout.isatty():
        parser.error("internal pipe-only operation")
    try:
        if args.command in ("use", "login"):
            verify_native_guard_manifest()
        print(json.dumps(run(args.command, args.target), ensure_ascii=False))
    except Exception as exc:  # noqa: BLE001 - security boundary: never echo credential-bearing errors
        # AccountError codes are constructed internally; arbitrary exceptions may contain secrets.
        code = str(exc) if isinstance(exc, AccountError) else type(exc).__name__
        print(json.dumps({"error": code}), file=sys.stderr)
        raise SystemExit(1)


if __name__ == "__main__":
    main()
