#!/usr/bin/env python3
"""
Codex Proxy — запускает xray-core как локальный HTTP-прокси для Codex
через VLESS-сервер на VPS или через VLESS-подписку.

Зачем:
  - корпоративный VPN продолжает работать для корпоративных ресурсов;
  - Codex ходит через локальный HTTP proxy 127.0.0.1:10810;
  - xray запускается как обычный процесс, а не как системный VPN.

Использование:
  codex-proxy                    # выбрать сервер и запустить xray
  codex-proxy --set-vless         # вставить один vless:// конфиг
  codex-proxy --set-sub           # сохранить URL подписки и обновить серверы
  codex-proxy --update            # обновить список серверов из подписки
  codex-proxy --allow-insecure-sub --update
                                 # аварийно обновить подписку без TLS-проверки сертификата
  codex-proxy --list              # показать все серверы
  codex-proxy --status            # проверить процесс и OpenAI через proxy
  codex-proxy --install-codex-env # записать proxy env в ~/.codex/.env
  codex-proxy --print-env         # вывести export-команды для текущего shell
  codex-proxy --run [args...]     # запустить codex CLI с proxy env
  codex-proxy --run-app           # запустить Codex.app через macOS launchctl + open
  codex-proxy --unset-app-env     # удалить proxy env из launchctl для GUI-приложений
  codex-proxy --stop              # остановить xray

Примеры:
  codex-proxy --set-vless
  codex-proxy
  codex-proxy --status
  codex-proxy --allow-insecure-sub --update
  codex-proxy --install-codex-env
  codex-proxy --run
  codex-proxy --run exec "ping"
"""

from __future__ import annotations

import base64
import json
import os
import shutil
import signal
import subprocess
import sys
import time
from pathlib import Path
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import parse_qs, unquote

CONFIG_DIR = Path.home() / ".config" / "xray-codex"
SUB_FILE = CONFIG_DIR / "subscription_url.txt"
SERVERS_FILE = CONFIG_DIR / "servers.json"
XRAY_CONFIG = CONFIG_DIR / "config.json"
PID_FILE = CONFIG_DIR / "xray.pid"
CODEX_DIR = Path.home() / ".codex"
CODEX_ENV_FILE = CODEX_DIR / ".env"

# Используем отдельный порт, чтобы не конфликтовать с Claude/V2Box/V2RayTun.
LOCAL_HTTP_PORT = int(os.environ.get("CODEX_PROXY_PORT", "10810"))
LOCAL_HTTP_URL = f"http://127.0.0.1:{LOCAL_HTTP_PORT}"
NO_PROXY_VALUE = "127.0.0.1,localhost,::1,*.local"
ALLOW_INSECURE_SUB_FLAG = "--allow-insecure-sub"
SECURE_DIR_MODE = 0o700
SECURE_FILE_MODE = 0o600


def chmod_if_possible(path: Path, mode: int) -> None:
    """Best-effort chmod for secret-bearing files and directories."""
    try:
        path.chmod(mode)
    except FileNotFoundError:
        return
    except PermissionError as exc:
        print(f"Warning: cannot chmod {path}: {exc}", file=sys.stderr)
    except OSError as exc:
        print(f"Warning: chmod failed for {path}: {exc}", file=sys.stderr)


def ensure_dirs() -> None:
    CONFIG_DIR.mkdir(parents=True, exist_ok=True)
    chmod_if_possible(CONFIG_DIR, SECURE_DIR_MODE)


def secure_write_text(path: Path, content: str) -> None:
    """Atomically write text and restrict permissions to the current user only."""
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.parent == CONFIG_DIR:
        chmod_if_possible(path.parent, SECURE_DIR_MODE)

    tmp_path = path.with_name(f".{path.name}.tmp")
    fd = os.open(tmp_path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, SECURE_FILE_MODE)
    chmod_if_possible(tmp_path, SECURE_FILE_MODE)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            handle.write(content)
        os.replace(tmp_path, path)
        chmod_if_possible(path, SECURE_FILE_MODE)
    except Exception:
        try:
            tmp_path.unlink(missing_ok=True)
        finally:
            raise


def harden_existing_secret_files() -> None:
    """Restrict permissions for files that may contain VLESS/config/proxy secrets."""
    ensure_dirs()
    for path in (SUB_FILE, SERVERS_FILE, XRAY_CONFIG, PID_FILE, CODEX_ENV_FILE):
        if path.exists():
            chmod_if_possible(path, SECURE_FILE_MODE)


def die(message: str, exit_code: int = 1) -> None:
    print(message, file=sys.stderr)
    sys.exit(exit_code)


def save_subscription_url(url: str) -> None:
    ensure_dirs()
    url = url.strip()
    if not url:
        die("Subscription URL cannot be empty.")
    secure_write_text(SUB_FILE, url + "\n")
    print("Subscription URL saved.")


def get_subscription_url() -> str:
    ensure_dirs()
    if SUB_FILE.exists():
        return SUB_FILE.read_text(encoding="utf-8").strip()

    print("First run — enter your VLESS subscription URL:")
    print("If you have only one VLESS config, use: codex-proxy --set-vless")
    url = input("> ").strip()
    if not url:
        die("URL cannot be empty.")
    save_subscription_url(url)
    return url


def decode_subscription_payload(raw: bytes) -> str:
    """Decode subscription payload. Some providers return plain text, others base64."""
    text = raw.decode("utf-8", errors="ignore").strip()
    if "vless://" in text:
        return text

    compact = "".join(text.split())
    padding = "=" * (-len(compact) % 4)
    try:
        decoded = base64.b64decode(compact + padding, validate=False)
        return decoded.decode("utf-8", errors="ignore")
    except Exception:
        return text


def allow_insecure_subscription_tls(args: list[str] | None = None) -> bool:
    """Return True only for an explicit insecure subscription download override."""
    args = args or []
    return ALLOW_INSECURE_SUB_FLAG in args or truthy(
        os.environ.get("CODEX_PROXY_ALLOW_INSECURE_SUB", "0")
    )


def fetch_subscription(url: str, *, allow_insecure_tls: bool = False) -> list[str]:
    """Fetch subscription and return vless:// links. TLS verification is enabled by default."""
    import ssl
    import urllib.request

    ctx = ssl.create_default_context()
    if allow_insecure_tls:
        # Explicit fallback only for broken/self-signed subscription endpoints.
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
        print(
            "Warning: TLS certificate verification is DISABLED for subscription download. "
            "Use this only for trusted self-hosted subscription URLs.",
            file=sys.stderr,
        )

    req = urllib.request.Request(url, headers={"User-Agent": "V2Box/1.0"})
    with urllib.request.urlopen(req, context=ctx, timeout=20) as resp:
        raw = resp.read()

    decoded = decode_subscription_payload(raw)
    return [
        line.strip()
        for line in decoded.splitlines()
        if line.strip().startswith("vless://")
    ]


def parse_vless_url(url: str) -> dict[str, Any] | None:
    """Parse vless://uuid@host:port?params#name into a dict usable for xray."""
    try:
        url = url.strip()
        if not url.startswith("vless://"):
            return None

        if "#" in url:
            url_part, name = url.rsplit("#", 1)
            name = unquote(name) or "VLESS server"
        else:
            url_part, name = url, "VLESS server"

        without_scheme = url_part[len("vless://") :]
        uuid_part, rest = without_scheme.split("@", 1)

        if "?" in rest:
            host_port, query_str = rest.split("?", 1)
        else:
            host_port, query_str = rest, ""

        if host_port.startswith("["):
            host_end = host_port.index("]")
            host = host_port[1:host_end]
            port = int(host_port[host_end + 2 :])
        else:
            parts = host_port.rsplit(":", 1)
            host = parts[0]
            port = int(parts[1]) if len(parts) > 1 else 443

        params_raw = parse_qs(query_str)
        params: dict[str, Any] = {
            k: v[0] if len(v) == 1 else v for k, v in params_raw.items()
        }

        return {
            "name": name,
            "uuid": uuid_part,
            "address": host,
            "port": port,
            "type": params.get("type", "tcp"),
            "security": params.get("security", "none"),
            "sni": params.get("sni", ""),
            "host": params.get("host", ""),
            "path": params.get("path", ""),
            "flow": params.get("flow", ""),
            "fp": params.get("fp", ""),
            "pbk": params.get("pbk", ""),
            "sid": params.get("sid", ""),
            "spx": params.get("spx", ""),
            "alpn": params.get("alpn", ""),
            "encryption": params.get("encryption", "none"),
            "headerType": params.get("headerType", ""),
            "serviceName": params.get("serviceName", ""),
            "mode": params.get("mode", ""),
            "allowInsecure": params.get("allowInsecure", "0"),
            "raw_url": url,
        }
    except Exception as exc:
        print(f"Warning: failed to parse VLESS URL: {url[:80]}... ({exc})")
        return None


def save_servers(servers: list[dict[str, Any]]) -> None:
    ensure_dirs()
    secure_write_text(
        SERVERS_FILE, json.dumps(servers, ensure_ascii=False, indent=2) + "\n"
    )


def set_single_vless() -> list[dict[str, Any]]:
    print("Paste your full vless:// config URL:")
    link = input("> ").strip()
    parsed = parse_vless_url(link)
    if not parsed:
        die("Invalid VLESS URL. It must start with vless://")
    save_servers([parsed])
    print(f"Saved 1 VLESS server: {parsed['name']}")
    return [parsed]


def update_servers(*, allow_insecure_tls: bool = False) -> list[dict[str, Any]]:
    url = get_subscription_url()
    print("Fetching subscription...")
    links = fetch_subscription(url, allow_insecure_tls=allow_insecure_tls)
    print(f"Found {len(links)} VLESS link(s).")

    servers = [parsed for link in links if (parsed := parse_vless_url(link))]
    save_servers(servers)
    print(f"Saved {len(servers)} server(s) to cache: {SERVERS_FILE}")
    return servers


def load_servers() -> list[dict[str, Any]]:
    ensure_dirs()
    if SERVERS_FILE.exists():
        return json.loads(SERVERS_FILE.read_text(encoding="utf-8"))
    return update_servers()


def display_servers(servers: list[dict[str, Any]]) -> None:
    print(
        f"\n{'#':>3}  {'Name':<45} {'Address':<30} {'Port':>5}  {'Transport':<10} {'Security':<10}"
    )
    print("-" * 115)
    for i, s in enumerate(servers, 1):
        address = s.get("address", "")
        addr_display = address[:27] + "..." if len(address) > 30 else address
        print(
            f"{i:>3}  {s.get('name', ''):<45} "
            f"{addr_display:<30} {s.get('port', ''):>5}  "
            f"{s.get('type', ''):<10} {s.get('security', ''):<10}"
        )


def truthy(value: Any) -> bool:
    return str(value).lower() in {"1", "true", "yes"}


def generate_stream_settings(server: dict[str, Any]) -> dict[str, Any]:
    network = server.get("type", "tcp") or "tcp"
    security = server.get("security", "none") or "none"
    stream: dict[str, Any] = {"network": network}

    if security == "tls":
        stream["security"] = "tls"
        tls_settings: dict[str, Any] = {}
        if server.get("sni"):
            tls_settings["serverName"] = server["sni"]
        if server.get("fp"):
            tls_settings["fingerprint"] = server["fp"]
        if server.get("alpn"):
            tls_settings["alpn"] = str(server["alpn"]).split(",")
        if truthy(server.get("allowInsecure")):
            tls_settings["allowInsecure"] = True
        stream["tlsSettings"] = tls_settings

    elif security == "reality":
        stream["security"] = "reality"
        reality_settings: dict[str, Any] = {}
        if server.get("sni"):
            reality_settings["serverName"] = server["sni"]
        if server.get("fp"):
            reality_settings["fingerprint"] = server["fp"]
        if server.get("pbk"):
            reality_settings["publicKey"] = server["pbk"]
        if server.get("sid"):
            reality_settings["shortId"] = server["sid"]
        if server.get("spx"):
            reality_settings["spiderX"] = server["spx"]
        stream["realitySettings"] = reality_settings

    if network == "ws":
        ws_settings: dict[str, Any] = {}
        if server.get("path"):
            ws_settings["path"] = server["path"]
        if server.get("host"):
            ws_settings["headers"] = {"Host": server["host"]}
        stream["wsSettings"] = ws_settings

    elif network == "grpc":
        grpc_settings: dict[str, Any] = {}
        if server.get("serviceName"):
            grpc_settings["serviceName"] = server["serviceName"]
        if server.get("mode"):
            grpc_settings["multiMode"] = server["mode"] == "multi"
        stream["grpcSettings"] = grpc_settings

    elif network == "tcp" and server.get("headerType") == "http":
        stream["tcpSettings"] = {
            "header": {
                "type": "http",
                "request": {
                    "path": [server.get("path") or "/"],
                    "headers": {
                        "Host": [server.get("host") or server.get("sni") or ""]
                    },
                },
            }
        }

    elif network == "h2":
        http_settings: dict[str, Any] = {}
        if server.get("host"):
            http_settings["host"] = [server["host"]]
        if server.get("path"):
            http_settings["path"] = server["path"]
        stream["httpSettings"] = http_settings

    elif network == "httpupgrade":
        httpupgrade_settings: dict[str, Any] = {}
        if server.get("path"):
            httpupgrade_settings["path"] = server["path"]
        if server.get("host"):
            httpupgrade_settings["host"] = server["host"]
        stream["httpupgradeSettings"] = httpupgrade_settings

    elif network in {"xhttp", "splithttp"}:
        xhttp_settings: dict[str, Any] = {}
        if server.get("path"):
            xhttp_settings["path"] = server["path"]
        if server.get("host"):
            xhttp_settings["host"] = server["host"]
        if server.get("mode"):
            xhttp_settings["mode"] = server["mode"]
        stream["xhttpSettings"] = xhttp_settings

    return stream


def generate_xray_config(server: dict[str, Any]) -> dict[str, Any]:
    user: dict[str, Any] = {
        "id": server["uuid"],
        "encryption": server.get("encryption", "none") or "none",
    }
    if server.get("flow"):
        user["flow"] = server["flow"]

    return {
        "log": {"loglevel": "warning"},
        "inbounds": [
            {
                "port": LOCAL_HTTP_PORT,
                "listen": "127.0.0.1",
                "protocol": "http",
                "tag": "http-in",
                "settings": {"timeout": 300},
            }
        ],
        "outbounds": [
            {
                "protocol": "vless",
                "settings": {
                    "vnext": [
                        {
                            "address": server["address"],
                            "port": int(server["port"]),
                            "users": [user],
                        }
                    ]
                },
                "streamSettings": generate_stream_settings(server),
                "tag": "proxy",
            },
            {"protocol": "freedom", "tag": "direct"},
            {"protocol": "blackhole", "tag": "block"},
        ],
    }


def read_pid() -> int | None:
    if not PID_FILE.exists():
        return None
    try:
        return int(PID_FILE.read_text(encoding="utf-8").strip())
    except ValueError:
        PID_FILE.unlink(missing_ok=True)
        return None


def is_pid_running(pid: int) -> bool:
    try:
        os.kill(pid, 0)
        return True
    except ProcessLookupError:
        return False
    except PermissionError:
        return True


def get_process_command(pid: int) -> str:
    """Return process command line, or an empty string if it cannot be read."""
    try:
        result = subprocess.run(
            ["ps", "-p", str(pid), "-o", "command="],
            capture_output=True,
            text=True,
            check=False,
        )
        if result.returncode != 0:
            return ""
        return result.stdout.strip()
    except OSError:
        return ""


def is_xray_codex_process(pid: int) -> bool:
    """Verify that PID points to the xray process started with this script's config."""
    command = get_process_command(pid)
    if not command:
        return False

    normalized = command.replace("\\ ", " ")
    executable = normalized.split(maxsplit=1)[0] if normalized else ""
    executable_name = Path(executable).name.lower()
    return executable_name.startswith("xray") and str(XRAY_CONFIG) in normalized


def is_codex_proxy_running() -> bool:
    pid = read_pid()
    return bool(pid and is_pid_running(pid) and is_xray_codex_process(pid))


def ensure_xray_available() -> str:
    xray_bin = shutil.which("xray")
    if not xray_bin:
        die("xray not found. Install it with: brew install xray")
    return xray_bin


def start_xray(server: dict[str, Any]) -> None:
    ensure_dirs()
    existing_pid = read_pid()
    if existing_pid and is_pid_running(existing_pid):
        if is_xray_codex_process(existing_pid):
            print(f"xray is already running for Codex (PID {existing_pid}).")
            print("Stop it first: codex-proxy --stop")
            return
        print(
            f"Warning: PID file points to PID {existing_pid}, but it is not this codex xray process. "
            "Removing stale PID file without stopping that process.",
            file=sys.stderr,
        )
        PID_FILE.unlink(missing_ok=True)
    elif existing_pid:
        PID_FILE.unlink(missing_ok=True)

    xray_bin = ensure_xray_available()
    config = generate_xray_config(server)
    secure_write_text(
        XRAY_CONFIG, json.dumps(config, ensure_ascii=False, indent=2) + "\n"
    )

    print(f"\nConfig saved: {XRAY_CONFIG}")
    print(f"Server: {server['name']}")
    print(f"Address: {server['address']}:{server['port']}")
    print(f"Transport: {server.get('type')}, Security: {server.get('security')}")

    proc = subprocess.Popen(
        [xray_bin, "run", "-c", str(XRAY_CONFIG)],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.PIPE,
    )

    time.sleep(1.5)

    if proc.poll() is not None:
        stderr = (
            proc.stderr.read().decode("utf-8", errors="replace") if proc.stderr else ""
        )
        die(f"\nxray failed to start:\n{stderr}")

    secure_write_text(PID_FILE, str(proc.pid) + "\n")
    print(f"\nxray started (PID {proc.pid})")
    print(f"HTTP proxy: {LOCAL_HTTP_URL}")
    print("\nTest it:")
    print(
        f"  curl -x {LOCAL_HTTP_URL} -s -o /dev/null -w 'HTTP Status: %{{http_code}}\\n' https://api.openai.com/v1/models"
    )
    print(
        "Expected: HTTP 401 without API key. 401 is OK here: it means proxy reached OpenAI."
    )
    print("\nFor Codex CLI:")
    print("  codex-proxy --run")
    print("or install env once:")
    print("  codex-proxy --install-codex-env")


def stop_xray() -> None:
    pid = read_pid()
    if not pid:
        print("xray is not running for Codex (no PID file).")
        return

    if not is_pid_running(pid):
        print(f"xray process {pid} not found (already stopped).")
        PID_FILE.unlink(missing_ok=True)
        return

    if not is_xray_codex_process(pid):
        print(
            f"Refusing to stop PID {pid}: it is not the xray process started by codex-proxy.",
            file=sys.stderr,
        )
        print("Removing stale PID file without stopping that process.", file=sys.stderr)
        PID_FILE.unlink(missing_ok=True)
        return

    try:
        os.kill(pid, signal.SIGTERM)
        print(f"xray stopped (PID {pid}).")
    except ProcessLookupError:
        print(f"xray process {pid} not found (already stopped).")
    PID_FILE.unlink(missing_ok=True)


def open_via_proxy(url: str, timeout: int = 8) -> tuple[bool, str]:
    import urllib.request

    proxy = urllib.request.ProxyHandler(
        {"https": LOCAL_HTTP_URL, "http": LOCAL_HTTP_URL}
    )
    opener = urllib.request.build_opener(proxy)
    req = urllib.request.Request(url, headers={"User-Agent": "codex-proxy-check/1.0"})
    try:
        with opener.open(req, timeout=timeout) as resp:
            return True, f"HTTP {resp.status}"
    except HTTPError as exc:
        # 401/403/404 still prove that TLS + proxy path reached the remote endpoint.
        if 100 <= exc.code < 500:
            return True, f"HTTP {exc.code}"
        return False, f"HTTP {exc.code}"
    except (URLError, TimeoutError, OSError) as exc:
        return False, str(exc)


def check_status() -> None:
    pid = read_pid()
    running = bool(pid and is_pid_running(pid) and is_xray_codex_process(pid))

    if running:
        print(f"xray is running for Codex (PID {pid})")
    else:
        print("xray is NOT running for Codex")
        if pid and is_pid_running(pid):
            print(
                f"PID file points to PID {pid}, but it is not this codex xray process.",
                file=sys.stderr,
            )
            PID_FILE.unlink(missing_ok=True)
        elif pid:
            PID_FILE.unlink(missing_ok=True)

    for url in [
        "https://api.openai.com/v1/models",
        "https://auth.openai.com/",
        "https://chatgpt.com/",
    ]:
        ok, result = open_via_proxy(url)
        status = "OK" if ok else "FAIL"
        print(f"Proxy check {status}: {url} -> {result}")

    if XRAY_CONFIG.exists():
        cfg = json.loads(XRAY_CONFIG.read_text(encoding="utf-8"))
        vnext = cfg.get("outbounds", [{}])[0].get("settings", {}).get("vnext", [{}])[0]
        print(f"Current server: {vnext.get('address')}:{vnext.get('port')}")


def codex_proxy_env() -> dict[str, str]:
    return {
        "HTTP_PROXY": LOCAL_HTTP_URL,
        "HTTPS_PROXY": LOCAL_HTTP_URL,
        "ALL_PROXY": LOCAL_HTTP_URL,
        "http_proxy": LOCAL_HTTP_URL,
        "https_proxy": LOCAL_HTTP_URL,
        "all_proxy": LOCAL_HTTP_URL,
        "WS_PROXY": LOCAL_HTTP_URL,
        "WSS_PROXY": LOCAL_HTTP_URL,
        "ws_proxy": LOCAL_HTTP_URL,
        "wss_proxy": LOCAL_HTTP_URL,
        "NO_PROXY": NO_PROXY_VALUE,
        "no_proxy": NO_PROXY_VALUE,
    }


def print_env() -> None:
    for key, value in codex_proxy_env().items():
        print(f'export {key}="{value}"')


def install_codex_env() -> None:
    CODEX_DIR.mkdir(parents=True, exist_ok=True)

    managed_start = "# >>> codex-proxy managed block >>>"
    managed_end = "# <<< codex-proxy managed block <<<"
    block_lines = [managed_start]
    for key, value in codex_proxy_env().items():
        block_lines.append(f'{key}="{value}"')
    block_lines.append(managed_end)
    block = "\n".join(block_lines) + "\n"

    existing = (
        CODEX_ENV_FILE.read_text(encoding="utf-8") if CODEX_ENV_FILE.exists() else ""
    )
    if managed_start in existing and managed_end in existing:
        before, rest = existing.split(managed_start, 1)
        _, after = rest.split(managed_end, 1)
        new_content = before.rstrip() + "\n\n" + block + after.lstrip()
    else:
        prefix = existing.rstrip() + "\n\n" if existing.strip() else ""
        new_content = prefix + block

    secure_write_text(CODEX_ENV_FILE, new_content)
    print(f"Codex env written: {CODEX_ENV_FILE}")
    print("Restart Codex CLI/IDE extension after this change.")
    print(
        "If Codex.app was opened from Finder and ignores .env, use: codex-proxy --run-app"
    )


def require_proxy_running() -> None:
    pid = read_pid()
    if not (pid and is_pid_running(pid) and is_xray_codex_process(pid)):
        die("xray proxy is not running. Start it first: codex-proxy")


def run_codex_cli(codex_args: list[str]) -> int:
    require_proxy_running()
    codex_bin = shutil.which("codex")
    if not codex_bin:
        die(
            "codex CLI not found. Install it first: npm install -g @openai/codex or brew install --cask codex"
        )

    env = os.environ.copy()
    env.update(codex_proxy_env())
    print(f"Starting Codex CLI through proxy: {LOCAL_HTTP_URL}")
    return subprocess.run([codex_bin, *codex_args], env=env).returncode


def find_codex_app_bundle() -> Path:
    """Find Codex.app bundle. CODEX_APP_PATH may point either to .app or to the inner executable."""
    candidates: list[Path] = []

    env_path = os.environ.get("CODEX_APP_PATH", "").strip()
    if env_path:
        raw = Path(env_path).expanduser()
        candidates.append(raw)
        if raw.name == "Codex" and "Contents/MacOS" in str(raw):
            # /Applications/Codex.app/Contents/MacOS/Codex -> /Applications/Codex.app
            try:
                candidates.append(raw.parents[2])
            except IndexError:
                pass

    candidates.extend(
        [
            Path("/Applications/Codex.app"),
            Path.home() / "Applications" / "Codex.app",
        ]
    )

    for candidate in candidates:
        if candidate.exists() and candidate.suffix == ".app":
            return candidate

    checked = "\n".join(f"  - {c}" for c in candidates)
    die(
        "Codex.app bundle not found. Install Codex Desktop first, or set CODEX_APP_PATH to the .app bundle.\n"
        f"Checked:\n{checked}\n\n"
        "Example:\n"
        '  CODEX_APP_PATH="/Applications/Codex.app" codex-proxy --run-app'
    )
    raise AssertionError("unreachable")


def set_launchctl_proxy_env() -> None:
    """Set proxy env for macOS GUI apps launched after this point."""
    if sys.platform != "darwin":
        die("--run-app is supported only on macOS.")

    print(
        "Warning: launchctl proxy env affects GUI apps started after this command. "
        "Run 'codex-proxy --unset-app-env' after finishing Codex Desktop work.",
        file=sys.stderr,
    )
    for key, value in codex_proxy_env().items():
        subprocess.run(["launchctl", "setenv", key, value], check=True)


def unset_launchctl_proxy_env() -> None:
    """Remove proxy env previously set for macOS GUI apps."""
    if sys.platform != "darwin":
        die("--unset-app-env is supported only on macOS.")

    for key in codex_proxy_env().keys():
        subprocess.run(["launchctl", "unsetenv", key], check=False)
    print("Removed Codex proxy env from launchctl for future GUI app launches.")
    print("Already running apps may need to be restarted.")


def run_codex_app() -> int:
    require_proxy_running()
    app_bundle = find_codex_app_bundle()

    # Do not execute Contents/MacOS/Codex directly. Codex Desktop is an Electron app;
    # launching the inner binary from a terminal can crash with write EIO on stdout/stderr.
    # launchctl setenv + open starts the .app the normal macOS way while still giving it proxy env.
    set_launchctl_proxy_env()

    print(f"Starting Codex.app through proxy: {LOCAL_HTTP_URL}")
    print(f"App bundle: {app_bundle}")
    print("If Codex was already open, quit it first and run this command again.")
    print("After finishing, remove GUI proxy env with: codex-proxy --unset-app-env")
    print("Then stop xray with: codex-proxy --stop")

    return subprocess.run(["open", "-na", str(app_bundle)]).returncode


def select_and_start() -> None:
    servers = load_servers()
    if not servers:
        die("No servers found. Use codex-proxy --set-vless or codex-proxy --set-sub.")

    display_servers(servers)
    print(f"\nSelect server (1-{len(servers)}):")

    try:
        choice = int(input("> ").strip())
    except (ValueError, EOFError):
        die("Invalid selection.")

    if choice < 1 or choice > len(servers):
        die(f"Choose between 1 and {len(servers)}.")

    start_xray(servers[choice - 1])


def main() -> None:
    args = sys.argv[1:]
    ensure_dirs()
    harden_existing_secret_files()

    if "--help" in args or "-h" in args:
        print(__doc__)
        return

    if "--set-vless" in args:
        set_single_vless()
        return

    if "--set-sub" in args:
        url = input("Enter subscription URL: ").strip()
        save_subscription_url(url)
        update_servers(allow_insecure_tls=allow_insecure_subscription_tls(args))
        return

    if "--update" in args:
        update_servers(allow_insecure_tls=allow_insecure_subscription_tls(args))
        return

    if "--stop" in args:
        stop_xray()
        return

    if "--status" in args:
        check_status()
        return

    if "--install-codex-env" in args:
        install_codex_env()
        return

    if "--print-env" in args:
        print_env()
        return

    if "--list" in args:
        display_servers(load_servers())
        return

    if "--unset-app-env" in args:
        unset_launchctl_proxy_env()
        return

    if "--run-app" in args:
        raise SystemExit(run_codex_app())

    if "--run" in args:
        run_index = args.index("--run")
        codex_args = args[run_index + 1 :]
        if codex_args and codex_args[0] == "--":
            codex_args = codex_args[1:]
        raise SystemExit(run_codex_cli(codex_args))

    select_and_start()


if __name__ == "__main__":
    main()
