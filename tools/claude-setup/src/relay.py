import asyncio
import base64
import fcntl
import hashlib
import hmac
import ipaddress
import json
import os
from pathlib import Path
import re
import signal
import socket
import ssl
import stat
import sys
from urllib.parse import unquote

from guard import validate_proxy
from install_local import open_private


def validate_direct_hosts(value):
    if not isinstance(value, (list, tuple)) or len(value) > 32:
        raise ValueError("Invalid direct HTTPS hosts")
    hosts = []
    for host in value:
        if not isinstance(host, str) or len(host) > 253 or not host.isascii():
            raise ValueError("Invalid direct HTTPS hosts")
        host = host.lower()
        labels = host.split(".")
        if len(labels) < 2 or any(not re.fullmatch(r"[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?", label) for label in labels):
            raise ValueError("Invalid direct HTTPS hosts")
        try:
            ipaddress.ip_address(host)
        except ValueError:
            pass
        else:
            raise ValueError("Invalid direct HTTPS hosts")
        if labels[-1].isdigit():
            raise ValueError("Invalid direct HTTPS hosts")
        hosts.append(host)
    return tuple(sorted(set(hosts)))


def fingerprint(url, direct_hosts=()):
    hosts = validate_direct_hosts(direct_hosts)
    data = url.encode() if not hosts else json.dumps(["claude-guard-direct-v1", url, hosts], separators=(",", ":")).encode()
    return hashlib.sha256(data).hexdigest()


def parse_request(header):
    lines = header[:-4].split(b"\r\n")
    if not lines or not re.fullmatch(rb"[!#$%&'*+.^_`|~0-9A-Za-z-]+ [!-~]+ HTTP/1\.[01]", lines[0]):
        raise ValueError("Invalid request")
    method, target, version = lines[0].split(b" ")
    fields = {}
    for line in lines[1:]:
        if not re.fullmatch(rb"[!#$%&'*+.^_`|~0-9A-Za-z-]+:[\x09\x20-\x7e]*", line):
            raise ValueError("Invalid request")
        name, value = line.split(b":", 1)
        name = name.lower()
        if name in fields and name in (b"host", b"proxy-authorization", b"content-length", b"transfer-encoding"):
            raise ValueError("Invalid request")
        fields[name] = value.strip(b" \t")
    if method != b"CONNECT":
        return None, fields
    match = re.fullmatch(rb"([A-Za-z0-9.-]+|\[[0-9A-Fa-f:.]+\]):([0-9]{1,5})", target)
    if not match:
        raise ValueError("Invalid request")
    if not 1 <= int(match.group(2)) <= 65535:
        raise ValueError("Invalid request")
    if match.group(2) != b"443":
        return None, fields
    authority = match.group(1).decode("ascii")
    if authority.startswith("["):
        try:
            ipaddress.IPv6Address(authority[1:-1])
        except ValueError:
            raise ValueError("Invalid request") from None
        return None, fields
    try:
        ipaddress.IPv4Address(authority)
    except ValueError:
        pass
    else:
        return None, fields
    if "." not in authority and re.fullmatch(r"[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?", authority):
        return None, fields
    try:
        host = validate_direct_hosts([match.group(1).decode("ascii")])[0]
    except ValueError:
        raise ValueError("Invalid request") from None
    if ((version == b"HTTP/1.1" and b"host" not in fields)
            or (b"host" in fields and fields[b"host"].lower() != target.lower()) or b"transfer-encoding" in fields
            or fields.get(b"content-length", b"0") != b"0"):
        raise ValueError("Invalid request")
    return host, fields


async def direct_connection(host):
    loop = asyncio.get_running_loop()
    answers = await loop.getaddrinfo(host, 443, type=socket.SOCK_STREAM)
    if not answers:
        raise OSError("No addresses")
    for family, _, _, _, address in answers:
        if family not in (socket.AF_INET, socket.AF_INET6):
            raise OSError("Unsafe address")
        ip = ipaddress.ip_address(address[0])
        if (ip.is_loopback or ip.is_link_local or ip.is_unspecified or ip.is_multicast
                or ip.is_reserved or getattr(ip, "ipv4_mapped", None)):
            raise OSError("Unsafe address")
    for family, _, _, _, address in answers:
        try:
            return await asyncio.open_connection(address[0], 443, family=family, flags=socket.AI_NUMERICHOST)
        except OSError:
            continue
    raise OSError("Connection failed")


def bind_loopback():
    for _ in range(16):
        ipv4 = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        ipv6 = socket.socket(socket.AF_INET6, socket.SOCK_STREAM)
        try:
            ipv6.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_V6ONLY, 1)
            ipv4.bind(("127.0.0.1", 0))
            port = ipv4.getsockname()[1]
            ipv6.bind(("::1", port))
            for listener in (ipv4, ipv6):
                listener.listen(128)
                listener.setblocking(False)
            return port, [ipv4, ipv6]
        except OSError:
            ipv4.close()
            ipv6.close()
    raise RuntimeError("Cannot reserve both loopback addresses for the proxy relay")


async def serve(url, state, direct_hosts=()):
    proxy = validate_proxy(url)
    direct_hosts = validate_direct_hosts(direct_hosts)
    state = Path(state)
    info = state.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise RuntimeError("Relay state must be an owned private directory")
    lock = open_private(state / "service.lock")
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    port, listeners = bind_loopback()
    active = set()
    stopped = asyncio.Event()
    tls = ssl.create_default_context() if proxy.scheme == "https" else None

    async def forward(reader, writer):
        while True:
            data = await asyncio.wait_for(reader.read(65536), 600)
            if not data:
                return
            writer.write(data)
            await asyncio.wait_for(writer.drain(), 30)

    async def client(reader, writer):
        task = asyncio.current_task()
        if len(active) >= 128:
            writer.close()
            return
        active.add(task)
        upstream = None
        pumps = []
        try:
            header = b""
            host = None
            if direct_hosts:
                try:
                    header = await asyncio.wait_for(reader.readuntil(b"\r\n\r\n"), 10)
                    if len(header) > 16384:
                        raise ValueError("Invalid request")
                    host, fields = parse_request(header)
                except (ValueError, asyncio.LimitOverrunError, asyncio.IncompleteReadError, asyncio.TimeoutError):
                    writer.write(b"HTTP/1.1 400 Bad Request\r\nConnection: close\r\n\r\n")
                    await writer.drain()
                    return
            if host in direct_hosts:
                if proxy.username is not None:
                    credentials = unquote(proxy.username) + ":" + unquote(proxy.password or "")
                    expected = b"Basic " + base64.b64encode(credentials.encode())
                    if not hmac.compare_digest(fields.get(b"proxy-authorization", b""), expected):
                        writer.write(b"HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic realm=\"proxy\"\r\nConnection: close\r\n\r\n")
                        await writer.drain()
                        return
                try:
                    incoming, upstream = await asyncio.wait_for(direct_connection(host), 15)
                except (OSError, asyncio.TimeoutError):
                    writer.write(b"HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n")
                    await writer.drain()
                    return
                writer.write(b"HTTP/1.1 200 Connection Established\r\n\r\n")
                await writer.drain()
            else:
                incoming, upstream = await asyncio.wait_for(
                    asyncio.open_connection(proxy.hostname, proxy.port, family=socket.AF_INET,
                                            ssl=tls, server_hostname=proxy.hostname if tls else None),
                    15,
                )
                if header:
                    upstream.write(header)
                    await upstream.drain()
            pumps = [asyncio.create_task(forward(reader, upstream)),
                     asyncio.create_task(forward(incoming, writer))]
            await asyncio.wait(pumps, return_when=asyncio.FIRST_COMPLETED)
        except (OSError, asyncio.TimeoutError, ssl.SSLError):
            pass
        finally:
            for pump in pumps:
                pump.cancel()
            if pumps:
                await asyncio.gather(*pumps, return_exceptions=True)
            writer.close()
            if upstream:
                upstream.close()
            active.discard(task)

    async def control(reader, writer):
        try:
            command = await asyncio.wait_for(reader.readline(), 2)
            if command == b"status\n":
                response = {"port": port, "fingerprint": fingerprint(url, direct_hosts), "pid": os.getpid()}
                writer.write(json.dumps(response).encode() + b"\n")
                await writer.drain()
            elif command == b"stop\n":
                stopped.set()
        except (OSError, ValueError, asyncio.TimeoutError):
            pass
        finally:
            writer.close()

    control_path = state / "control.sock"
    if control_path.exists() or control_path.is_symlink():
        info = control_path.lstat()
        if not stat.S_ISSOCK(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
            raise RuntimeError("Refusing foreign relay control path")
        control_path.unlink()
    servers = [await asyncio.start_server(client, sock=item, limit=16384) for item in listeners]
    servers.append(await asyncio.start_unix_server(control, path=str(control_path), limit=64))
    os.chmod(control_path, 0o600)
    loop = asyncio.get_running_loop()
    for sig in (signal.SIGTERM, signal.SIGINT):
        loop.add_signal_handler(sig, stopped.set)
    try:
        await stopped.wait()
    finally:
        for server in servers:
            server.close()
            await server.wait_closed()
        for task in list(active):
            task.cancel()
        await asyncio.gather(*list(active), return_exceptions=True)
        control_path.unlink(missing_ok=True)
        lock.close()


if __name__ == "__main__":
    os.umask(0o077)
    try:
        request = json.loads(sys.stdin.buffer.readline(32768))
        asyncio.run(serve(request["proxy_url"], sys.argv[1], request.get("direct_https_hosts", [])))
    except (OSError, ValueError, RuntimeError, KeyError):
        print("Proxy relay failed closed. Check proxy settings and relay ownership.", file=sys.stderr)
        sys.exit(1)
