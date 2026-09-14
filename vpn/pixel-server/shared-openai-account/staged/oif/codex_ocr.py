"""Access-only Hermes credential consumer; never refreshes or persists credentials."""

from __future__ import annotations

import asyncio
import base64
import json
import math
import re
import time
from pathlib import Path
from typing import Any
from uuid import UUID

import httpx

CODEX_URL = "https://chatgpt.com/backend-api/codex/responses"
MAX_STREAM_BYTES = 1024 * 1024


class CodexAuthError(RuntimeError):
    def __init__(self) -> None:
        super().__init__("OCR authorization unavailable")


class CodexResponseError(RuntimeError):
    def __init__(self) -> None:
        super().__init__("OCR response unavailable")


def _headers(path: Path) -> dict[str, str]:
    # These are identity guards for a trusted host-projected credential, not JWT
    # signature verification. The fixed OpenAI endpoint validates the signature.
    # Metadata and credentials come from one atomic root-owned snapshot read.
    # Never reopen it while preparing the headers for an in-flight request.
    try:
        with path.open("rb") as source:
            raw = source.read(65537)
        if len(raw) > 65536:
            raise ValueError
        auth = json.loads(raw)
        if auth.get("auth_mode") != "chatgptAuthTokens" or auth.get("OPENAI_API_KEY"):
            raise ValueError
        account = auth["hermes_account"]
        if type(account["schema_version"]) is not int or account["schema_version"] != 1:
            raise ValueError
        generation = account["generation"]
        account_id, email = account["account_id"], account["email"]
        if (
            not isinstance(generation, str)
            or not isinstance(account_id, str)
            or not account_id.strip()
            or not isinstance(email, str)
            or not email.strip()
            or not account_id.isascii()
            or not account_id.isprintable()
            or any(character.isspace() for character in account_id)
        ):
            raise ValueError
        UUID(generation)
        tokens = auth["tokens"]
        token = tokens["access_token"]
        if not isinstance(token, str) or not re.fullmatch(
            r"[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+", token
        ):
            raise ValueError
        segment = token.split(".")[1]
        claims = json.loads(base64.urlsafe_b64decode(segment + "=" * (-len(segment) % 4)))
        expiry = claims["exp"]
        if (
            isinstance(expiry, bool)
            or not isinstance(expiry, (int, float))
            or not math.isfinite(expiry)
            or expiry < time.time() + 90
            or claims["https://api.openai.com/auth"]["chatgpt_account_id"] != account_id
            or claims["https://api.openai.com/profile"]["email"] != email
            or tokens["account_id"] != account_id
            or tokens.get("refresh_token", "") != ""
        ):
            raise ValueError
        return {
            "Authorization": f"Bearer {token}",
            "ChatGPT-Account-ID": account_id,
            "originator": "codex_cli_rs",
            "User-Agent": "codex_cli_rs/0.145.0",
            "Accept": "text/event-stream",
        }
    except (OSError, ValueError, KeyError, TypeError, OverflowError, AttributeError):
        raise CodexAuthError from None


async def codex_response(
    client: httpx.AsyncClient, path: Path, body: dict[str, Any]
) -> dict[str, Any]:
    headers = await asyncio.to_thread(_headers, path)
    raw = bytearray()
    # A total deadline also bounds servers that trickle bytes indefinitely.
    async with asyncio.timeout(90):
        async with client.stream(
            "POST", CODEX_URL, json=body, headers=headers, follow_redirects=False
        ) as response:
            response.raise_for_status()
            async for chunk in response.aiter_bytes():
                if len(raw) + len(chunk) > MAX_STREAM_BYTES:
                    raise CodexResponseError
                raw.extend(chunk)
    try:
        stream = raw.decode("utf-8").replace("\r\n", "\n")
        if not stream.endswith("\n\n"):
            raise ValueError
        completed: dict[str, Any] | None = None
        done_items: dict[int, dict[str, Any]] = {}
        for event in stream.split("\n\n"):
            data = "\n".join(
                line[5:].lstrip() for line in event.splitlines() if line.startswith("data:")
            )
            if not data or data == "[DONE]":
                continue
            payload = json.loads(data)
            kind = payload["type"]
            if kind in {"error", "response.failed", "response.incomplete"}:
                raise ValueError
            if isinstance(kind, str) and ("refusal" in kind or "function_call" in kind):
                raise ValueError
            if kind in {"response.output_item.added", "response.output_item.done"}:
                item = payload["item"]
                if item["type"] not in {"message", "reasoning"}:
                    raise ValueError
                if item["type"] == "message" and (
                    item.get("role") != "assistant"
                    or any(
                        block.get("type") == "refusal" or "refusal" in block
                        for block in item.get("content", [])
                    )
                ):
                    raise ValueError
            if kind == "response.output_item.done":
                index = payload["output_index"]
                if not isinstance(index, int) or isinstance(index, bool) or index < 0:
                    raise ValueError
                if index in done_items:
                    raise ValueError
                done_items[index] = payload["item"]
            if kind == "response.completed":
                if completed is not None:
                    raise ValueError
                completed = payload["response"]
        if completed is None or completed.get("status") != "completed":
            raise ValueError
        output = completed["output"]
        if not isinstance(output, list):
            raise ValueError
        if not output:
            output = [done_items[index] for index in sorted(done_items)]
            completed["output"] = output
        if not output:
            raise ValueError
        for item in output:
            if not isinstance(item, dict) or item.get("type") not in {"message", "reasoning"}:
                raise ValueError
            if item["type"] == "message" and (
                item.get("role") != "assistant" or item.get("status") != "completed"
            ):
                raise ValueError
        return completed
    except (ValueError, TypeError, KeyError, AttributeError):
        raise CodexResponseError from None
