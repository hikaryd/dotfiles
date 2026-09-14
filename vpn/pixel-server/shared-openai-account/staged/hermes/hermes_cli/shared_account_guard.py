"""Pure disk-boundary guards for explicitly managed shared Codex accounts."""

from __future__ import annotations

import base64
import copy
import json
import uuid
from typing import Any

PROVIDER = "openai-codex"


class SharedAccountGuardError(RuntimeError):
    def __init__(self) -> None:
        super().__init__("Shared Codex account changed or is invalid")


def _policy(store: dict[str, Any]) -> dict[str, Any] | None:
    try:
        policy = store["shared_account"]
        if not isinstance(policy, dict):
            return None
        if type(policy["schema_version"]) is not int or policy["schema_version"] != 1:
            return None
        uuid.UUID(policy["generation"])
        for key in ("account_id", "email"):
            value = policy[key]
            if (
                not isinstance(value, str)
                or not value.strip()
                or any(ord(character) < 32 for character in value)
            ):
                return None
        return policy
    except (KeyError, TypeError, ValueError, AttributeError):
        return None


def _matches_identity(entry: Any, policy: dict[str, Any]) -> bool:
    if not isinstance(entry, dict) or entry.get("source") != "device_code":
        return False
    try:
        token = entry["access_token"]
        if (
            not isinstance(token, str)
            or len(token) > 65536
            or len(token.split(".")) != 3
        ):
            return False
        segment = token.split(".")[1]
        claims = json.loads(
            base64.urlsafe_b64decode(segment + "=" * (-len(segment) % 4))
        )
        return bool(
            claims["https://api.openai.com/auth"]["chatgpt_account_id"]
            == policy["account_id"]
            and claims["https://api.openai.com/profile"]["email"] == policy["email"]
        )
    except (KeyError, TypeError, ValueError, AttributeError):
        return False


def _same_pair(left: dict[str, Any], right: dict[str, Any]) -> bool:
    return all(
        left.get(key) == right.get(key) for key in ("access_token", "refresh_token")
    )


def shared_account_canonical_entry(store: dict[str, Any]) -> dict[str, Any] | None:
    """Read the canonical row only when it agrees with the singleton and policy."""
    policy = _policy(store)
    if policy is None or store.get("active_provider") != PROVIDER:
        return None
    try:
        tokens = store["providers"][PROVIDER]["tokens"]
        rows = store["credential_pool"][PROVIDER]
        if not isinstance(rows, list) or len(rows) != 1:
            return None
        row = rows[0]
        if not isinstance(row, dict):
            return None
        if (
            not _matches_identity(row, policy)
            or not isinstance(row.get("id"), str)
            or not row["id"]
            or not _same_pair(row, tokens)
            or not isinstance(tokens.get("refresh_token"), str)
            or not tokens["refresh_token"]
        ):
            return None
        return row
    except (KeyError, TypeError, AttributeError):
        return None


def filter_shared_account_pool_entries(
    provider: str,
    entries: list[Any],
    store: dict[str, Any],
) -> list[Any]:
    """Keep only the authoritative pair, normalizing to its single canonical ID.

    Runs under the native store lock against a fresh snapshot. Decoding JWTs is
    only an identity guard for trusted cached credentials, not signature checking.
    An invalid *present* policy fails closed; no policy/other providers are unchanged.
    """
    if provider != PROVIDER or "shared_account" not in store:
        return entries
    canonical = shared_account_canonical_entry(store)
    policy = _policy(store)
    if canonical is None or policy is None:
        return []
    for entry in entries:
        if _matches_identity(entry, policy) and _same_pair(entry, canonical):
            return [{**entry, "id": canonical["id"]}]
    return []


def prepare_shared_account_rotation(
    store: dict[str, Any],
    previous: dict[str, Any],
    updated: dict[str, Any],
) -> dict[str, Any]:
    """CAS the pre-POST pair; return singleton+row for one atomic native save."""
    canonical = shared_account_canonical_entry(store)
    policy = _policy(store)
    if (
        canonical is None
        or policy is None
        or not _same_pair(previous, canonical)
        or not _matches_identity(previous, policy)
        or not _matches_identity(updated, policy)
        or not isinstance(updated.get("refresh_token"), str)
        or not updated["refresh_token"]
    ):
        raise SharedAccountGuardError
    result = copy.deepcopy(store)
    state = result["providers"][PROVIDER]
    state["tokens"].update(
        {key: updated[key] for key in ("access_token", "refresh_token")}
    )
    if updated.get("last_refresh"):
        state["last_refresh"] = updated["last_refresh"]
    result["credential_pool"][PROVIDER] = [{**updated, "id": canonical["id"]}]
    return result
