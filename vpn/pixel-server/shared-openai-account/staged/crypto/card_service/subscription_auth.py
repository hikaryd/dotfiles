"""Read one explicitly routed access token from the Hermes auth store.

The Hermes store remains the sole refresh owner.  This module opens it read-only,
keeps credentials in memory only, and never repairs, refreshes, copies, logs, or
persists tokens.  JWT claims are used only to select and freshness-check the
required account; the fixed upstream HTTPS endpoint authenticates the bearer.
"""

import base64
import json
import math
import os
import stat
import time
import uuid
from pathlib import Path
from typing import Any, Iterator

AUTH_PATH = Path("/root/.hermes/auth.json")
MAX_AUTH_BYTES = 2 * 1024 * 1024
PROTON_ACCOUNT_ID = os.environ.get("DOTS_PROTON_ACCOUNT_ID", "").strip()
PM_ACCOUNT_ID = os.environ.get("DOTS_PM_ACCOUNT_ID", "").strip()
DEVELOPMENT_ACCOUNT_ID = PROTON_ACCOUNT_ID
# Temporary owner-selected production account.  The deployment stage remains
# production; only its explicitly versioned service-local account policy changes.
PRODUCTION_ACCOUNT_ID = PROTON_ACCOUNT_ID
DEVELOPMENT_ACCOUNT_POLICY = "development-proton-v1"
PRODUCTION_ACCOUNT_POLICY = "production-proton-owner-override-v1"
SHARED_ACCOUNT_POLICY = "production-hermes-shared-v1"
SHARED_ACCOUNT_ROUTE = "shared_account"


def is_shared_route(stage, account_policy, expected_account_id):
    return (stage, account_policy, expected_account_id) == (
        "production",
        SHARED_ACCOUNT_POLICY,
        SHARED_ACCOUNT_ROUTE,
    )


_STAGE_ROUTES = {
    "development": (DEVELOPMENT_ACCOUNT_POLICY, DEVELOPMENT_ACCOUNT_ID),
    "production": (PRODUCTION_ACCOUNT_POLICY, PRODUCTION_ACCOUNT_ID),
}


class AuthUnavailable(RuntimeError):
    def __init__(self) -> None:
        super().__init__("auth_unavailable")


def _validate_route(
    stage: str | None,
    account_policy: str | None,
    expected_account_id: str | None,
) -> str:
    """Require the exact deployment-stage/policy/account triple."""
    if (
        not isinstance(stage, str)
        or not isinstance(account_policy, str)
        or not isinstance(expected_account_id, str)
        or not expected_account_id.strip()
    ):
        raise AuthUnavailable()
    if not is_shared_route(
        stage, account_policy, expected_account_id
    ) and _STAGE_ROUTES.get(stage) != (account_policy, expected_account_id):
        raise AuthUnavailable()
    return expected_account_id


def _provider(document: object) -> dict[str, Any]:
    if not isinstance(document, dict):
        return {}
    providers = document.get("providers")
    if not isinstance(providers, dict):
        return {}
    provider = providers.get("openai-codex")
    return provider if isinstance(provider, dict) else {}


def _candidate_tokens(document: object) -> Iterator[object]:
    """Yield pool entries first, then the current provider token.

    Account identity is checked by the caller for every candidate, so pool order
    cannot route a request to a different account.
    """
    if isinstance(document, dict):
        pools = document.get("credential_pool")
        if isinstance(pools, dict):
            pool = pools.get("openai-codex")
            if isinstance(pool, list):
                for entry in pool:
                    if isinstance(entry, dict):
                        yield entry.get("access_token")

    tokens = _provider(document).get("tokens")
    if isinstance(tokens, dict):
        yield tokens.get("access_token")


def _claims(token: object) -> dict[str, Any] | None:
    if not isinstance(token, str) or not token or len(token) > 65536:
        return None
    parts = token.split(".")
    if len(parts) != 3:
        return None
    try:
        payload = base64.urlsafe_b64decode(parts[1] + "=" * (-len(parts[1]) % 4))
        claims = json.loads(payload)
    except Exception:
        return None
    return claims if isinstance(claims, dict) else None


def _matches(token: object, expected_account_id: str, minimum_expiry: float) -> bool:
    claims = _claims(token)
    if claims is None:
        return False
    expiry = claims.get("exp")
    auth = claims.get("https://api.openai.com/auth")
    account_id = auth.get("chatgpt_account_id") if isinstance(auth, dict) else None
    return (
        not isinstance(expiry, bool)
        and isinstance(expiry, (int, float))
        and math.isfinite(expiry)
        and expiry > minimum_expiry
        and account_id == expected_account_id
    )


def token_account_id(token: object) -> str:
    claims = _claims(token)
    auth = claims.get("https://api.openai.com/auth") if claims else None
    account_id = auth.get("chatgpt_account_id") if isinstance(auth, dict) else None
    if (
        not isinstance(account_id, str)
        or not account_id
        or any(ord(c) < 32 for c in account_id)
    ):
        raise AuthUnavailable()
    return account_id


def _shared_token(document: object, minimum_expiry: float) -> str:
    policy = document.get("shared_account") if isinstance(document, dict) else None
    if (
        not isinstance(policy, dict)
        or type(policy.get("schema_version")) is not int
        or policy["schema_version"] != 1
    ):
        raise AuthUnavailable()
    generation = policy.get("generation")
    if not isinstance(generation, str):
        raise AuthUnavailable()
    try:
        uuid.UUID(generation)
    except ValueError:
        raise AuthUnavailable() from None
    account_id, email = policy.get("account_id"), policy.get("email")
    if not isinstance(account_id, str) or not isinstance(email, str):
        raise AuthUnavailable()
    if any(
        not isinstance(v, str) or not v.strip() or any(ord(c) < 32 for c in v)
        for v in (account_id, email)
    ):
        raise AuthUnavailable()
    tokens = _provider(document).get("tokens")
    token = tokens.get("access_token") if isinstance(tokens, dict) else None
    if not isinstance(token, str) or not _matches(token, account_id, minimum_expiry):
        raise AuthUnavailable()
    claims = _claims(token)
    if claims is None:
        raise AuthUnavailable()
    profile = claims.get("https://api.openai.com/profile")
    token_email = (
        profile.get("email") if isinstance(profile, dict) else claims.get("email")
    )
    if token_email != email:
        raise AuthUnavailable()
    return token


def read_access_token(
    path: str | os.PathLike[str] = AUTH_PATH,
    margin: float = 120,
    *,
    stage: str | None = None,
    account_policy: str | None = None,
    expected_account_id: str | None = None,
) -> str:
    """Return a fresh token only for the explicit stage/account route.

    Missing route inputs, unknown stages, stage/account mismatches, an unsafe
    store, and absent/expired/malformed matching credentials all fail closed.
    Credentials for another account are never returned as fallback.
    """
    try:
        account_id = _validate_route(stage, account_policy, expected_account_id)
        if (
            isinstance(margin, bool)
            or not isinstance(margin, (int, float))
            or not math.isfinite(margin)
            or margin < 0
        ):
            raise AuthUnavailable()

        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC | os.O_NONBLOCK)
        with os.fdopen(fd, "rb") as stream:
            info = os.fstat(stream.fileno())
            if (
                not stat.S_ISREG(info.st_mode)
                or info.st_uid != os.geteuid()
                or info.st_mode & 0o077
            ):
                raise AuthUnavailable()
            raw = stream.read(MAX_AUTH_BYTES + 1)
        if len(raw) > MAX_AUTH_BYTES:
            raise AuthUnavailable()

        document = json.loads(raw)
        minimum_expiry = time.time() + margin
        if is_shared_route(stage, account_policy, expected_account_id):
            return _shared_token(document, minimum_expiry)
        for candidate in _candidate_tokens(document):
            if isinstance(candidate, str) and _matches(
                candidate, account_id, minimum_expiry
            ):
                return candidate
        raise AuthUnavailable()
    except Exception:
        raise AuthUnavailable() from None
