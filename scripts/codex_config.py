#!/usr/bin/env python3
"""Safely sync and apply the user-owned subset of Codex config.toml.

Codex stores durable preferences and runtime state in the same TOML file.  This
helper deliberately owns only the stable personal settings listed below and
preserves project trust, hook trust, plugin/OMX state, and UI state in place.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import stat
import subprocess
import tempfile
import tomllib
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Callable, Iterable, Mapping
from urllib.parse import parse_qsl, unquote, urlsplit


ROOT_KEYS = (
    "model",
    "model_provider",
    "personality",
    "suppress_unstable_features_warning",
    "service_tier",
    "model_reasoning_effort",
)

MANAGED_TABLE_PREFIXES = (
    ("shell_environment_policy",),
    ("mcp_servers",),
    ("agents",),
    ("model_providers",),
)

PARTIAL_TABLE_KEYS = {("tui",): ("status_line",)}

# Literal MCP environment values must be explicitly reviewed as non-secret.
# Everything else under mcp_servers.<name>.env is exported as a placeholder.
SAFE_LITERAL_MCP_ENV_PATHS = {
    ("mcp_servers", "zai-mcp-server", "env", "Z_AI_MODE"),
}

SECRET_PATHS = {
    ("mcp_servers", "zai-mcp-server", "env", "Z_AI_API_KEY"): "Z_AI_API_KEY",
    ("model_providers", "cf-openai", "base_url"): "CF_OPENAI_BASE_URL",
}

PLACEHOLDER_RE = re.compile(r"\$\{([A-Za-z_][A-Za-z0-9_]*)\}")
LIKELY_SECRET_VALUE_RE = re.compile(
    r"(?:sk-[A-Za-z0-9_-]{12,}|gh[pousr]_[A-Za-z0-9]{20,}|Bearer\s+[A-Za-z0-9._~-]{12,})"
)
SENSITIVE_NAME_PARTS = {
    "auth",
    "authorization",
    "bearer",
    "cookie",
    "credential",
    "credentials",
    "key",
    "passwd",
    "password",
    "secret",
    "token",
}
ENV_REFERENCE_KEYS = {"bearer_token_env_var", "env_key"}
SAFE_NONSECRET_SENSITIVE_KEYS = {"requires_openai_auth"}
SENSITIVE_URL_QUERY_NAMES = {
    "assertion",
    "code",
    "jwt",
    "oauth_code",
    "samlresponse",
    "session",
    "sig",
    "signature",
    "state",
    "ticket",
    "x-amz-signature",
    "x-goog-signature",
}
PROBE_KEY = "__dots_codex_table_probe_7fdd74d1__"
ROOT_MARKER_START = "# >>> dots Codex managed root >>>"
ROOT_MARKER_END = "# <<< dots Codex managed root <<<"
TABLE_MARKER_START = "# >>> dots Codex managed table >>>"
TABLE_MARKER_END = "# <<< dots Codex managed table <<<"
PARTIAL_MARKER_START = "# >>> dots Codex managed keys >>>"
PARTIAL_MARKER_END = "# <<< dots Codex managed keys <<<"


@dataclass
class TableBlock:
    path: tuple[str, ...]
    lines: list[str]

    def text(self) -> str:
        return "\n".join(self.lines)


@dataclass
class Document:
    root_lines: list[str]
    tables: list[TableBlock]


@dataclass
class TomlLexState:
    multiline_delimiter: str | None = None
    square_depth: int = 0
    curly_depth: int = 0

    def at_document_level(self) -> bool:
        return (
            self.multiline_delimiter is None
            and self.square_depth == 0
            and self.curly_depth == 0
        )


@dataclass
class ApplyResult:
    changed: bool
    backup: Path | None
    missing_groups: tuple[str, ...]
    preserved_missing_groups: tuple[str, ...]


@dataclass
class RemoveResult:
    changed: bool
    removed_groups: tuple[str, ...]


def _find_probe_path(
    value: object, path: tuple[str, ...] = ()
) -> tuple[str, ...] | None:
    if isinstance(value, dict):
        if PROBE_KEY in value:
            return path
        for key, child in value.items():
            found = _find_probe_path(child, path + (str(key),))
            if found is not None:
                return found
    elif isinstance(value, list):
        for child in value:
            found = _find_probe_path(child, path)
            if found is not None:
                return found
    return None


def _table_path(line: str) -> tuple[str, ...] | None:
    stripped = line.strip()
    if not stripped.startswith("["):
        return None
    try:
        parsed = tomllib.loads(f"{stripped}\n{PROBE_KEY} = true\n")
    except tomllib.TOMLDecodeError:
        return None
    return _find_probe_path(parsed)


def _is_escaped(text: str, index: int) -> bool:
    backslashes = 0
    index -= 1
    while index >= 0 and text[index] == "\\":
        backslashes += 1
        index -= 1
    return backslashes % 2 == 1


def _scan_toml_line(line: str, state: TomlLexState) -> None:
    """Track lexical contexts in which a table-looking line is only data."""

    index = 0
    while index < len(line):
        if state.multiline_delimiter is not None:
            delimiter = state.multiline_delimiter
            end = line.find(delimiter, index)
            while end >= 0 and delimiter == '"""' and _is_escaped(line, end):
                end = line.find(delimiter, end + 1)
            if end < 0:
                return
            state.multiline_delimiter = None
            index = end + len(delimiter)
            continue

        if line.startswith('"""', index) or line.startswith("'''", index):
            state.multiline_delimiter = line[index : index + 3]
            index += 3
            continue

        char = line[index]
        if char == "#":
            return
        if char == '"':
            index += 1
            while index < len(line):
                if line[index] == '"' and not _is_escaped(line, index):
                    index += 1
                    break
                index += 1
            continue
        if char == "'":
            closing_quote = line.find("'", index + 1)
            index = len(line) if closing_quote < 0 else closing_quote + 1
            continue
        if char == "[":
            state.square_depth += 1
        elif char == "]":
            state.square_depth = max(0, state.square_depth - 1)
        elif char == "{":
            state.curly_depth += 1
        elif char == "}":
            state.curly_depth = max(0, state.curly_depth - 1)
        index += 1


def parse_document(text: str) -> Document:
    root: list[str] = []
    tables: list[TableBlock] = []
    current: TableBlock | None = None
    lex_state = TomlLexState()

    for line in text.splitlines():
        path = _table_path(line) if lex_state.at_document_level() else None
        if path is not None:
            current = TableBlock(path=path, lines=[line])
            tables.append(current)
        elif current is None:
            root.append(line)
        else:
            current.lines.append(line)
        _scan_toml_line(line, lex_state)
    return Document(root_lines=root, tables=tables)


def _document_level_lines(lines: Iterable[str]) -> Iterable[tuple[str, bool]]:
    state = TomlLexState()
    for line in lines:
        at_document_level = state.at_document_level()
        yield line, at_document_level
        _scan_toml_line(line, state)


def _has_document_level_line(lines: Iterable[str], expected: str) -> bool:
    return any(
        at_document_level and line.strip() == expected
        for line, at_document_level in _document_level_lines(lines)
    )


def _has_document_level_marker_pair(lines: Iterable[str], start: str, end: str) -> bool:
    saw_start = False
    for line, at_document_level in _document_level_lines(lines):
        if not at_document_level:
            continue
        stripped = line.strip()
        if stripped == start:
            saw_start = True
        elif stripped == end and saw_start:
            return True
    return False


def _starts_with(path: tuple[str, ...], prefix: tuple[str, ...]) -> bool:
    return path[: len(prefix)] == prefix


def is_managed_table(path: tuple[str, ...]) -> bool:
    return any(_starts_with(path, prefix) for prefix in MANAGED_TABLE_PREFIXES)


def table_group(path: tuple[str, ...]) -> tuple[str, ...]:
    if path and path[0] in {"mcp_servers", "model_providers"} and len(path) >= 2:
        return path[:2]
    return path[:1]


def format_group(group: tuple[str, ...]) -> str:
    return ".".join(group)


def validate_toml(text: str, label: str) -> dict[str, object]:
    try:
        return tomllib.loads(text)
    except tomllib.TOMLDecodeError as exc:
        raise ValueError(f"{label}: invalid TOML: {exc}") from exc


def validate_template_scope(document: Document, parsed: Mapping[str, object]) -> None:
    root_text = "\n".join(document.root_lines)
    root_values = (
        validate_toml(root_text, "Codex template root") if root_text.strip() else {}
    )
    unexpected_root = sorted(set(root_values).difference(ROOT_KEYS))
    unexpected_tables = sorted(
        format_group(block.path)
        for block in document.tables
        if not is_managed_table(block.path) and block.path not in PARTIAL_TABLE_KEYS
    )
    unexpected_partial_keys: list[str] = []
    for path, allowed_keys in PARTIAL_TABLE_KEYS.items():
        node: object = parsed
        for part in path:
            if not isinstance(node, dict) or part not in node:
                node = None
                break
            node = node[part]
        if isinstance(node, dict):
            unexpected_partial_keys.extend(
                f"{format_group(path)}.{key}"
                for key in sorted(set(node).difference(allowed_keys))
            )

    if unexpected_root or unexpected_tables or unexpected_partial_keys:
        details: list[str] = []
        if unexpected_root:
            details.append("root keys: " + ", ".join(unexpected_root))
        if unexpected_tables:
            details.append("tables: " + ", ".join(unexpected_tables))
        if unexpected_partial_keys:
            details.append("partial keys: " + ", ".join(unexpected_partial_keys))
        raise ValueError(
            "template contains unmanaged settings (" + "; ".join(details) + ")"
        )


def _toml_value(value: object) -> str:
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, str):
        # JSON basic strings are compatible with TOML basic strings for the
        # scalar values managed here.
        return json.dumps(value, ensure_ascii=False)
    if isinstance(value, int):
        return str(value)
    if isinstance(value, float):
        return repr(value)
    if isinstance(value, list):
        return "[" + ", ".join(_toml_value(item) for item in value) + "]"
    if isinstance(value, dict):
        return (
            "{ "
            + ", ".join(
                f"{_toml_key(str(key))} = {_toml_value(item)}"
                for key, item in value.items()
            )
            + " }"
        )
    raise TypeError(f"unsupported root TOML value: {type(value).__name__}")


def _toml_key(key: str) -> str:
    if re.fullmatch(r"[A-Za-z0-9_-]+", key):
        return key
    return json.dumps(key, ensure_ascii=False)


def _table_header(path: tuple[str, ...]) -> str:
    return "[" + ".".join(_toml_key(part) for part in path) + "]"


def _sanitize_snapshot_value(
    value: object,
    path: tuple[str, ...],
    *,
    home: Path,
    placeholders: set[str],
) -> object:
    is_environment_reference = bool(
        path
        and (
            path[-1] in ENV_REFERENCE_KEYS
            or ("env_http_headers" in path and path[-1] != "env_http_headers")
            or (path[-1] == "env_vars" and not isinstance(value, list))
        )
    )
    if is_environment_reference:
        if not isinstance(value, str) or not re.fullmatch(
            r"[A-Za-z_][A-Za-z0-9_]*", value
        ):
            raise ValueError(
                "environment reference must be a shell variable name: "
                + format_group(path)
            )
        return value

    exact_secret = SECRET_PATHS.get(path)
    if exact_secret:
        if not isinstance(value, str):
            raise ValueError(
                f"configured secret value must be a string: {format_group(path)}"
            )
        placeholders.add(exact_secret)
        return f"${{{exact_secret}}}"
    if len(path) == 3 and path[0] == "shell_environment_policy" and path[1] == "set":
        variable = path[-1]
        if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", variable):
            raise ValueError(
                f"shell environment key is not a valid variable: {variable}"
            )
        if not isinstance(value, str):
            raise ValueError(f"shell environment value must be a string: {variable}")
        placeholders.add(variable)
        return f"${{{variable}}}"
    if len(path) == 4 and path[0] == "mcp_servers" and path[2] == "env":
        variable = path[-1]
        if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", variable):
            raise ValueError(
                f"MCP environment key is not a valid shell variable: {variable}"
            )
        if not isinstance(value, str):
            raise ValueError(f"MCP environment value must be a string: {variable}")
        if path in SAFE_LITERAL_MCP_ENV_PATHS:
            return value
        placeholders.add(variable)
        return f"${{{variable}}}"
    if isinstance(value, dict):
        return {
            str(key): _sanitize_snapshot_value(
                child, path + (str(key),), home=home, placeholders=placeholders
            )
            for key, child in value.items()
        }
    if isinstance(value, list):
        return [
            _sanitize_snapshot_value(child, path, home=home, placeholders=placeholders)
            for child in value
        ]
    if isinstance(value, str):
        home_text = str(home)
        return value.replace(home_text, "${HOME}") if home_text else value
    return value


def _serialize_table_tree(
    path: tuple[str, ...], values: Mapping[str, object]
) -> list[TableBlock]:
    scalar_lines: list[str] = []
    children: list[tuple[str, Mapping[str, object]]] = []
    for key, value in values.items():
        if isinstance(value, dict):
            children.append((str(key), value))
        else:
            scalar_lines.append(f"{_toml_key(str(key))} = {_toml_value(value)}")

    blocks: list[TableBlock] = []
    if scalar_lines or not children:
        blocks.append(TableBlock(path, [_table_header(path), *scalar_lines]))
    for key, child in children:
        blocks.extend(_serialize_table_tree(path + (key,), child))
    return blocks


def _join_parts(parts: Iterable[str]) -> str:
    nonempty = [part.strip("\n") for part in parts if part.strip()]
    return "\n\n".join(nonempty) + ("\n" if nonempty else "")


def _has_sensitive_name_part(name: str) -> bool:
    normalized = re.sub(r"([a-z0-9])([A-Z])", r"\1-\2", name).lower()
    ordered_parts = [part for part in re.split(r"[-_.]+", normalized) if part]
    parts = set(ordered_parts)
    compact = "".join(ordered_parts)
    return bool(parts.intersection(SENSITIVE_NAME_PARTS)) or any(
        marker in compact
        for marker in (
            "apikey",
            "accesstoken",
            "authtoken",
            "bearertoken",
            "privatekey",
            "sessioncookie",
        )
    )


def _has_sensitive_config_field(path: tuple[str, ...]) -> bool:
    if not path:
        return False
    if path[0] in {"mcp_servers", "model_providers"}:
        fields = path[2:]
    elif path[0] in {prefix[0] for prefix in MANAGED_TABLE_PREFIXES}:
        fields = path[1:]
    else:
        fields = path
    return any(
        not field.startswith("[")
        and field not in SAFE_NONSECRET_SENSITIVE_KEYS
        and _has_sensitive_name_part(field)
        for field in fields
    )


def _url_contains_static_credentials(value: str) -> bool:
    try:
        parsed = urlsplit(value)
    except ValueError:
        return True
    if not parsed.scheme or not parsed.netloc:
        return False
    if parsed.username is not None or parsed.password is not None:
        return True
    query_pairs = parse_qsl(parsed.query, keep_blank_values=True)
    fragment_pairs = parse_qsl(parsed.fragment, keep_blank_values=True)
    if any(
        (_has_sensitive_name_part(name) or name.lower() in SENSITIVE_URL_QUERY_NAMES)
        and not PLACEHOLDER_RE.fullmatch(query_value)
        for name, query_value in [*query_pairs, *fragment_pairs]
    ):
        return True

    segments = [unquote(segment) for segment in parsed.path.split("/") if segment]
    for index, segment in enumerate(segments):
        if PLACEHOLDER_RE.fullmatch(segment):
            continue
        segment_parts = {part for part in re.split(r"[-_.]+", segment.lower()) if part}
        if _has_sensitive_name_part(segment) or segment_parts.intersection(
            {"sig", "signature", "signing"}
        ):
            next_segment = segments[index + 1] if index + 1 < len(segments) else ""
            if not PLACEHOLDER_RE.fullmatch(next_segment):
                return True
    return False


def _placeholder_backed_assignment(value: str) -> bool:
    stripped = value.strip()
    if PLACEHOLDER_RE.fullmatch(stripped):
        return True
    assignment = re.match(r"^[A-Za-z_][A-Za-z0-9_.-]*\s*[:=]\s*(.+?)\s*$", stripped)
    return bool(assignment and PLACEHOLDER_RE.fullmatch(assignment.group(1)))


def _leak_gate(text: str, home: Path) -> None:
    parsed = validate_toml(text, "sanitized Codex snapshot")

    forbidden_roots = {
        "projects",
        "hooks",
        "plugins",
        "marketplaces",
        "features",
        "notice",
    }
    leaked_roots = sorted(forbidden_roots.intersection(parsed))
    if leaked_roots:
        raise ValueError(
            "runtime-owned tables leaked into snapshot: " + ", ".join(leaked_roots)
        )

    tui = parsed.get("tui")
    if isinstance(tui, dict):
        unexpected_tui = sorted(set(tui).difference(PARTIAL_TABLE_KEYS[("tui",)]))
        if unexpected_tui:
            raise ValueError(
                "runtime/unmanaged TUI keys leaked into snapshot: "
                + ", ".join(unexpected_tui)
            )

    problems: list[str] = []

    def walk(value: object, path: tuple[str, ...] = ()) -> None:
        if isinstance(value, dict):
            for key, child in value.items():
                walk(child, path + (str(key),))
            return
        if isinstance(value, list):
            for index, child in enumerate(value):
                walk(child, path + (f"[{index}]",))
            return
        if not isinstance(value, str):
            if path:
                is_env_reference = (
                    path[-1] in ENV_REFERENCE_KEYS or "env_http_headers" in path
                )
                dotted = ".".join(path)
                if not is_env_reference and _has_sensitive_config_field(path):
                    problems.append(f"non-string secret-like key at {dotted}")
                if "http_headers" in path:
                    problems.append(f"non-string static HTTP header at {dotted}")
            return

        dotted = ".".join(path)
        if str(home) and str(home) in value:
            problems.append(f"absolute home path at {dotted}")
        if LIKELY_SECRET_VALUE_RE.search(value):
            problems.append(f"secret-looking value at {dotted}")
        if path:
            is_env_reference = (
                path[-1] in ENV_REFERENCE_KEYS or "env_http_headers" in path
            )
            expected = SECRET_PATHS.get(path)
            if expected and value != f"${{{expected}}}":
                problems.append(f"unmasked configured secret at {dotted}")
            elif (
                len(path) >= 4
                and path[0] == "mcp_servers"
                and path[2] == "env"
                and path not in SAFE_LITERAL_MCP_ENV_PATHS
                and not PLACEHOLDER_RE.fullmatch(value)
            ):
                problems.append(f"unmasked MCP environment value at {dotted}")
            elif (
                len(path) == 3
                and path[0] == "shell_environment_policy"
                and path[1] == "set"
                and not PLACEHOLDER_RE.fullmatch(value)
            ):
                problems.append(f"unmasked shell environment value at {dotted}")
            elif (
                not is_env_reference
                and _has_sensitive_config_field(path)
                and not PLACEHOLDER_RE.fullmatch(value)
            ):
                problems.append(f"unmasked secret-like key at {dotted}")
            elif "http_headers" in path and not PLACEHOLDER_RE.fullmatch(value):
                problems.append(f"static HTTP header value at {dotted}")
            elif path[-1] in {"url", "base_url"} and _url_contains_static_credentials(
                value
            ):
                problems.append(f"credential-bearing URL at {dotted}")

    walk(parsed)

    mcp_servers = parsed.get("mcp_servers")
    if isinstance(mcp_servers, dict):
        for server_name, config in mcp_servers.items():
            if not isinstance(config, dict):
                continue
            args = config.get("args")
            if not isinstance(args, list):
                continue
            for index, arg in enumerate(args):
                if not isinstance(arg, str):
                    continue
                option_name = ""
                option_value: object = ""
                if arg.startswith("--"):
                    option_name, separator, inline_value = arg[2:].partition("=")
                    option_value = (
                        inline_value
                        if separator
                        else args[index + 1]
                        if index + 1 < len(args)
                        else ""
                    )
                elif arg.startswith("-H"):
                    option_name = "header"
                    option_value = (
                        arg[2:]
                        if len(arg) > 2
                        else args[index + 1]
                        if index + 1 < len(args)
                        else ""
                    )
                elif arg.startswith("-e"):
                    option_name = "env"
                    option_value = (
                        arg[2:]
                        if len(arg) > 2
                        else args[index + 1]
                        if index + 1 < len(args)
                        else ""
                    )
                elif arg.startswith("-"):
                    option_name = arg.lstrip("-")
                    option_value = args[index + 1] if index + 1 < len(args) else ""

                if _url_contains_static_credentials(arg):
                    problems.append(
                        f"credential-bearing URL in MCP arguments at mcp_servers.{server_name}.args[{index}]"
                    )
                if isinstance(option_value, str) and _url_contains_static_credentials(
                    option_value
                ):
                    problems.append(
                        f"credential-bearing MCP option value at mcp_servers.{server_name}.args[{index}]"
                    )

                assignment = re.match(
                    r"^([A-Za-z_][A-Za-z0-9_.-]*)\s*[:=]\s*(.+?)\s*$", arg
                )
                if (
                    assignment
                    and _has_sensitive_name_part(assignment.group(1))
                    and not PLACEHOLDER_RE.fullmatch(assignment.group(2))
                ):
                    problems.append(
                        f"secret-bearing assignment in MCP arguments at mcp_servers.{server_name}.args[{index}]"
                    )

                normalized_option = option_name.lower().replace("_", "-")
                is_header_or_env = normalized_option in {
                    "env",
                    "environment",
                    "header",
                    "headers",
                    "http-header",
                }
                if is_header_or_env and (
                    not isinstance(option_value, str)
                    or not _placeholder_backed_assignment(option_value)
                ):
                    problems.append(
                        f"static header/environment MCP argument at mcp_servers.{server_name}.args[{index}]"
                    )
                if (
                    option_name
                    and _has_sensitive_name_part(option_name)
                    and (
                        not isinstance(option_value, str)
                        or not PLACEHOLDER_RE.fullmatch(option_value)
                    )
                ):
                    problems.append(
                        f"secret-bearing MCP argument at mcp_servers.{server_name}.args[{index}]"
                    )

    # Snapshot text is generated rather than copied, so this also guards any
    # future generated comments or formatting paths.
    if LIKELY_SECRET_VALUE_RE.search(text):
        problems.append("secret-looking token present in generated snapshot text")
    if problems:
        raise ValueError("; ".join(sorted(set(problems))))


def _atomic_write(path: Path, text: str, mode: int) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp_name = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    tmp = Path(tmp_name)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            handle.write(text)
            handle.flush()
            os.fsync(handle.fileno())
        os.chmod(tmp, mode)
        os.replace(tmp, path)
    finally:
        if tmp.exists():
            tmp.unlink()


def _assert_file_unchanged(path: Path, existed: bool, original_text: str) -> None:
    if path.exists() != existed:
        raise ValueError(f"{path} changed while Codex config was being prepared")
    if existed and path.read_text(encoding="utf-8") != original_text:
        raise ValueError(f"{path} changed while Codex config was being prepared")


def validate_with_codex(text: str, codex_bin: Path) -> None:
    with tempfile.TemporaryDirectory(prefix="codex-config-validate-") as tmp:
        validation_home = Path(tmp)
        config_path = validation_home / "config.toml"
        config_path.write_text(text, encoding="utf-8")
        os.chmod(config_path, 0o600)
        env = os.environ.copy()
        env["CODEX_HOME"] = str(validation_home)
        try:
            result = subprocess.run(
                [str(codex_bin), "--strict-config", "app-server"],
                stdin=subprocess.DEVNULL,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                env=env,
                check=False,
                timeout=30,
            )
        except subprocess.TimeoutExpired as exc:
            raise ValueError("Codex strict config validation timed out") from exc
        if result.returncode != 0:
            # The rendered config can contain credentials. Do not forward
            # validator stderr because a schema error may echo the bad value.
            raise ValueError("Codex strict config validation failed")


def sync_config(
    source: Path, target: Path, *, home: Path
) -> tuple[bool, tuple[str, ...]]:
    if source.resolve() == target.resolve():
        raise ValueError(
            "Codex config source and snapshot target must be different files"
        )

    live_text = source.read_text(encoding="utf-8")
    parsed = validate_toml(live_text, str(source))

    root_lines = [
        f"{key} = {_toml_value(parsed[key])}" for key in ROOT_KEYS if key in parsed
    ]

    tables: list[TableBlock] = []
    placeholders: set[str] = set()
    for prefix in MANAGED_TABLE_PREFIXES:
        node: object = parsed
        for part in prefix:
            if not isinstance(node, dict) or part not in node:
                node = None
                break
            node = node[part]
        if not isinstance(node, dict):
            continue
        sanitized = _sanitize_snapshot_value(
            node, prefix, home=home, placeholders=placeholders
        )
        if not isinstance(sanitized, dict):
            raise TypeError(f"expected TOML table at {format_group(prefix)}")
        tables.extend(_serialize_table_tree(prefix, sanitized))

    # Partial tables are serialized from parsed values so only explicitly
    # owned keys reach the repository snapshot.  Unknown TUI/runtime siblings
    # remain exclusively in the live config.
    for path, keys in PARTIAL_TABLE_KEYS.items():
        node: object = parsed
        for part in path:
            if not isinstance(node, dict) or part not in node:
                node = None
                break
            node = node[part]
        if not isinstance(node, dict):
            continue
        assignments = [
            f"{key} = {_toml_value(node[key])}" for key in keys if key in node
        ]
        if assignments:
            tables.append(TableBlock(path, [f"[{'.'.join(path)}]", *assignments]))

    header = (
        "# Stable personal Codex settings managed by ~/dots.\n"
        "# Runtime/project/plugin state is intentionally preserved only in ~/.codex/config.toml."
    )
    snapshot = _join_parts(
        [header + "\n" + "\n".join(root_lines)] + [block.text() for block in tables]
    )
    placeholders.update(PLACEHOLDER_RE.findall(snapshot))
    _leak_gate(snapshot, home)

    target_existed = target.exists()
    previous = target.read_text(encoding="utf-8") if target_existed else None
    changed = previous != snapshot
    _assert_file_unchanged(source, True, live_text)
    _assert_file_unchanged(target, target_existed, previous or "")
    if changed:
        mode = stat.S_IMODE(target.stat().st_mode) if target_existed else 0o644
        _atomic_write(target, snapshot, mode)
    return changed, tuple(sorted(placeholders))


def _toml_string_inner(value: str) -> str:
    return json.dumps(value, ensure_ascii=False)[1:-1]


def _render_text(text: str, values: Mapping[str, str]) -> str:
    def replace(match: re.Match[str]) -> str:
        name = match.group(1)
        if name not in values or not values[name]:
            return match.group(0)
        return _toml_string_inner(values[name])

    return PLACEHOLDER_RE.sub(replace, text)


def _managed_root_without_existing(
    root_lines: list[str], *, remove_unmarked_keys: bool = True
) -> list[str]:
    cleaned: list[str] = []
    inside_managed_block = False
    lex_state = TomlLexState()
    index = 0
    while index < len(root_lines):
        line = root_lines[index]
        at_document_level = lex_state.at_document_level()
        if at_document_level and line.strip() == ROOT_MARKER_START:
            if inside_managed_block:
                raise ValueError(f"nested managed root marker: {ROOT_MARKER_START}")
            inside_managed_block = True
            _scan_toml_line(line, lex_state)
            index += 1
            continue
        if at_document_level and line.strip() == ROOT_MARKER_END:
            if inside_managed_block:
                inside_managed_block = False
                _scan_toml_line(line, lex_state)
                index += 1
                continue
        if inside_managed_block:
            _scan_toml_line(line, lex_state)
            index += 1
            continue

        if remove_unmarked_keys and at_document_level:
            candidate: list[str] = []
            end = index
            while end < len(root_lines):
                candidate.append(root_lines[end])
                try:
                    values = tomllib.loads("\n".join(candidate) + "\n")
                except tomllib.TOMLDecodeError:
                    end += 1
                    continue
                if set(values).intersection(ROOT_KEYS):
                    index = end + 1
                    break
                break
            if index > end:
                continue

        cleaned.append(line)
        _scan_toml_line(line, lex_state)
        index += 1
    if inside_managed_block:
        raise ValueError(f"unterminated managed root marker: {ROOT_MARKER_START}")
    return cleaned


def _without_table_assignments(block: TableBlock, keys: tuple[str, ...]) -> TableBlock:
    """Remove selected assignments, including multiline TOML values."""

    kept = [block.lines[0]]
    lex_state = TomlLexState()
    _scan_toml_line(block.lines[0], lex_state)
    index = 1
    while index < len(block.lines):
        line = block.lines[index]
        at_document_level = lex_state.at_document_level()
        if at_document_level and line.strip() in {
            PARTIAL_MARKER_START,
            PARTIAL_MARKER_END,
        }:
            _scan_toml_line(line, lex_state)
            index += 1
            continue
        remove_end: int | None = None
        if at_document_level:
            candidate: list[str] = []
            end = index
            while end < len(block.lines):
                candidate.append(block.lines[end])
                try:
                    values = tomllib.loads(
                        f"{block.lines[0]}\n" + "\n".join(candidate) + "\n"
                    )
                except tomllib.TOMLDecodeError:
                    end += 1
                    continue
                node: object = values
                for part in block.path:
                    if not isinstance(node, dict) or part not in node:
                        node = None
                        break
                    node = node[part]
                if isinstance(node, dict) and set(node).intersection(keys):
                    remove_end = end
                break

        if remove_end is None:
            kept.append(line)
            _scan_toml_line(line, lex_state)
            index += 1
            continue
        index = remove_end + 1
    return TableBlock(block.path, kept)


def _meaningful_table_body(lines: list[str]) -> list[str]:
    body = list(lines)
    while body and not body[0].strip():
        body.pop(0)
    while body and not body[-1].strip():
        body.pop()
    return body


def _mark_managed_table(block: TableBlock) -> TableBlock:
    return TableBlock(
        block.path,
        [block.lines[0], TABLE_MARKER_START, *block.lines[1:], TABLE_MARKER_END],
    )


def _mark_partial_table(block: TableBlock, extra: list[str]) -> TableBlock:
    return TableBlock(
        block.path,
        [
            block.lines[0],
            PARTIAL_MARKER_START,
            *block.lines[1:],
            PARTIAL_MARKER_END,
            *extra,
        ],
    )


def _next_backup_path(target: Path) -> Path:
    stamp = datetime.now().strftime("%Y%m%d%H%M%S")
    candidate = target.with_name(f"{target.name}.bak-{stamp}")
    suffix = 1
    while candidate.exists():
        candidate = target.with_name(f"{target.name}.bak-{stamp}.{suffix}")
        suffix += 1
    return candidate


def apply_config(
    template: Path,
    target: Path,
    *,
    values: Mapping[str, str],
    validator: Callable[[str], None] | None = None,
) -> ApplyResult:
    if template.resolve() == target.resolve():
        raise ValueError("Codex config template and target must be different files")

    template_text = template.read_text(encoding="utf-8")
    template_values = validate_toml(template_text, str(template))
    template_doc = parse_document(template_text)
    validate_template_scope(template_doc, template_values)

    missing_groups: set[tuple[str, ...]] = set()
    for block in template_doc.tables:
        missing = {
            name
            for name in PLACEHOLDER_RE.findall(block.text())
            if not values.get(name)
        }
        if missing:
            missing_groups.add(table_group(block.path))

    root_missing = {
        name
        for name in PLACEHOLDER_RE.findall("\n".join(template_doc.root_lines))
        if not values.get(name)
    }
    if root_missing:
        raise ValueError(
            "missing variables required by root Codex settings: "
            + ", ".join(sorted(root_missing))
        )

    rendered_root = _render_text("\n".join(template_doc.root_lines), values)
    rendered_tables = [
        TableBlock(block.path, _render_text(block.text(), values).splitlines())
        for block in template_doc.tables
        if table_group(block.path) not in missing_groups
    ]
    rendered_template = _join_parts(
        [rendered_root] + [block.text() for block in rendered_tables]
    )
    validate_toml(rendered_template, "rendered Codex template")

    live_existed = target.exists()
    live_text = target.read_text(encoding="utf-8") if live_existed else ""
    if live_text.strip():
        validate_toml(live_text, str(target))
    live_doc = parse_document(live_text)

    preserved_tables: list[TableBlock] = []
    preserved_missing_tables: dict[tuple[str, ...], list[TableBlock]] = {}
    partial_live: dict[tuple[str, ...], TableBlock] = {}
    preserved_missing: set[tuple[str, ...]] = set()
    for block in live_doc.tables:
        if block.path in PARTIAL_TABLE_KEYS:
            partial_live[block.path] = _without_table_assignments(
                block, PARTIAL_TABLE_KEYS[block.path]
            )
            continue
        if not is_managed_table(block.path):
            preserved_tables.append(block)
            continue
        group = table_group(block.path)
        if group in missing_groups:
            preserved_missing_tables.setdefault(group, []).append(block)
            preserved_missing.add(group)

    final_rendered_tables: list[TableBlock] = []
    rendered_index = 0
    emitted_missing_groups: set[tuple[str, ...]] = set()
    for template_block in template_doc.tables:
        group = table_group(template_block.path)
        if group in missing_groups:
            if group not in emitted_missing_groups:
                final_rendered_tables.extend(preserved_missing_tables.get(group, []))
                emitted_missing_groups.add(group)
            continue

        block = rendered_tables[rendered_index]
        rendered_index += 1
        if block.path not in PARTIAL_TABLE_KEYS:
            final_rendered_tables.append(
                _mark_managed_table(block) if is_managed_table(block.path) else block
            )
            continue
        live_block = partial_live.pop(block.path, None)
        extra = _meaningful_table_body(live_block.lines[1:]) if live_block else []
        final_rendered_tables.append(_mark_partial_table(block, extra))

    if rendered_index != len(rendered_tables):
        raise ValueError("rendered Codex table order did not match template")

    # If a managed partial key was removed from the template, remove that key
    # from the live table but retain every locally-owned sibling setting.
    for block in partial_live.values():
        body = _meaningful_table_body(block.lines[1:])
        if body:
            preserved_tables.append(TableBlock(block.path, [block.lines[0], *body]))

    managed_root = _join_parts(
        [ROOT_MARKER_START, rendered_root, ROOT_MARKER_END]
    ).rstrip()
    preserved_root = "\n".join(_managed_root_without_existing(live_doc.root_lines))
    merged = _join_parts(
        [
            managed_root,
            preserved_root,
            *[block.text() for block in preserved_tables],
            *[block.text() for block in final_rendered_tables],
        ]
    )
    validate_toml(merged, "merged Codex config")
    if validator is not None:
        validator(merged)

    _assert_file_unchanged(target, live_existed, live_text)

    # A live config may contain secrets by design; the leak gate is only for
    # the repository snapshot.  The rendered target is always private (0600).
    changed = merged != live_text
    backup: Path | None = None
    if changed:
        target.parent.mkdir(parents=True, exist_ok=True)
        if live_existed:
            backup = _next_backup_path(target)
            _atomic_write(backup, live_text, 0o600)
            _assert_file_unchanged(target, live_existed, live_text)
        _atomic_write(target, merged, 0o600)
    elif live_existed and stat.S_IMODE(target.stat().st_mode) != 0o600:
        _assert_file_unchanged(target, live_existed, live_text)
        os.chmod(target, 0o600)

    return ApplyResult(
        changed=changed,
        backup=backup,
        missing_groups=tuple(sorted(format_group(group) for group in missing_groups)),
        preserved_missing_groups=tuple(
            sorted(format_group(group) for group in preserved_missing)
        ),
    )


def remove_config(template: Path, target: Path) -> RemoveResult:
    """Remove only settings owned by the tracked template from a live config."""

    if template.resolve() == target.resolve():
        raise ValueError("Codex config template and target must be different files")

    template_text = template.read_text(encoding="utf-8")
    template_values = validate_toml(template_text, str(template))
    validate_template_scope(parse_document(template_text), template_values)

    if not target.exists():
        return RemoveResult(changed=False, removed_groups=())

    live_text = target.read_text(encoding="utf-8")
    validate_toml(live_text, str(target))
    live_doc = parse_document(live_text)

    preserved_tables: list[TableBlock] = []
    removed_groups: set[tuple[str, ...]] = set()
    for block in live_doc.tables:
        if block.path in PARTIAL_TABLE_KEYS and _has_document_level_marker_pair(
            block.lines, PARTIAL_MARKER_START, PARTIAL_MARKER_END
        ):
            cleaned = _without_table_assignments(block, PARTIAL_TABLE_KEYS[block.path])
            removed_groups.add(block.path)
            body = _meaningful_table_body(cleaned.lines[1:])
            if body:
                preserved_tables.append(
                    TableBlock(cleaned.path, [cleaned.lines[0], *body])
                )
            continue
        if is_managed_table(block.path) and _has_document_level_marker_pair(
            block.lines, TABLE_MARKER_START, TABLE_MARKER_END
        ):
            removed_groups.add(table_group(block.path))
            continue
        preserved_tables.append(block)

    root_was_managed = _has_document_level_marker_pair(
        live_doc.root_lines, ROOT_MARKER_START, ROOT_MARKER_END
    )
    if root_was_managed:
        removed_groups.add(("root",))
    if not removed_groups:
        return RemoveResult(changed=False, removed_groups=())

    preserved_root = "\n".join(
        _managed_root_without_existing(live_doc.root_lines, remove_unmarked_keys=False)
        if root_was_managed
        else live_doc.root_lines
    )
    cleaned_text = _join_parts(
        [preserved_root, *[block.text() for block in preserved_tables]]
    )
    if cleaned_text:
        validate_toml(cleaned_text, "Codex config after removing dotfiles settings")

    changed = cleaned_text != live_text
    if changed:
        _assert_file_unchanged(target, True, live_text)
        if cleaned_text:
            _atomic_write(target, cleaned_text, 0o600)
        else:
            target.unlink()

    return RemoveResult(
        changed=changed,
        removed_groups=tuple(sorted(format_group(group) for group in removed_groups)),
    )


def _cmd_sync(args: argparse.Namespace) -> int:
    changed, placeholders = sync_config(args.source, args.target, home=args.home)
    print(f"  config.toml: {'updated' if changed else 'unchanged'} -> {args.target}")
    if placeholders:
        print("  placeholders: " + ", ".join(placeholders))
    print("  secrets/runtime leak gate: passed")
    return 0


def _cmd_apply(args: argparse.Namespace) -> int:
    values = {name: value for name, value in os.environ.items() if value}
    validator: Callable[[str], None] | None = None
    if args.codex_bin is not None:

        def strict_validator(text: str) -> None:
            validate_with_codex(text, args.codex_bin)

        validator = strict_validator
    result = apply_config(
        args.template,
        args.target,
        values=values,
        validator=validator,
    )
    if args.codex_bin is not None:
        print("  Codex strict config validation: passed")
    if result.backup:
        print(f"  backup: {result.backup.name}")
    print(f"  config.toml: {'updated' if result.changed else 'unchanged'}")
    if result.missing_groups:
        print(
            "  missing secrets; skipped template groups: "
            + ", ".join(result.missing_groups)
        )
    if result.preserved_missing_groups:
        print(
            "  preserved existing secret-backed groups: "
            + ", ".join(result.preserved_missing_groups)
        )
    return 0


def _cmd_remove(args: argparse.Namespace) -> int:
    result = remove_config(args.template, args.target)
    print(
        f"  config.toml: {'managed settings removed' if result.changed else 'unchanged'}"
    )
    if result.removed_groups:
        print("  managed table groups: " + ", ".join(result.removed_groups))
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    sync_parser = subparsers.add_parser(
        "sync", help="export a sanitized stable snapshot"
    )
    sync_parser.add_argument("--source", type=Path, required=True)
    sync_parser.add_argument("--target", type=Path, required=True)
    sync_parser.add_argument("--home", type=Path, default=Path.home())
    sync_parser.set_defaults(handler=_cmd_sync)

    apply_parser = subparsers.add_parser(
        "apply", help="merge the stable snapshot into Codex home"
    )
    apply_parser.add_argument("--template", type=Path, required=True)
    apply_parser.add_argument("--target", type=Path, required=True)
    apply_parser.add_argument(
        "--codex-bin",
        type=Path,
        help="validate the merged config with this Codex executable before writing",
    )
    apply_parser.set_defaults(handler=_cmd_apply)

    remove_parser = subparsers.add_parser(
        "remove", help="remove only dotfiles-owned settings from Codex home"
    )
    remove_parser.add_argument("--template", type=Path, required=True)
    remove_parser.add_argument("--target", type=Path, required=True)
    remove_parser.set_defaults(handler=_cmd_remove)
    return parser


def main() -> int:
    args = build_parser().parse_args()
    try:
        return args.handler(args)
    except (OSError, TypeError, ValueError) as exc:
        print(f"codex-config: {exc}", file=os.sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
