import asyncio
import fcntl
import hashlib
import json
import os
from pathlib import Path
import signal
import socket
import ssl
import sys

from guard import validate_proxy


def fingerprint(url):
    return hashlib.sha256(url.encode()).hexdigest()


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


async def serve(url, state):
    proxy = validate_proxy(url)
    state = Path(state)
    lock = open(state / "service.lock", "a")
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
            # The peer never supplies a destination, including on reconnect.
            incoming, upstream = await asyncio.wait_for(
                asyncio.open_connection(proxy.hostname, proxy.port, family=socket.AF_INET,
                                        ssl=tls, server_hostname=proxy.hostname if tls else None),
                15,
            )
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
            command = await asyncio.wait_for(reader.read(64), 2)
            if command == b"status\n":
                response = {"port": port, "fingerprint": fingerprint(url), "pid": os.getpid()}
                writer.write(json.dumps(response).encode() + b"\n")
                await writer.drain()
            elif command == b"stop\n":
                stopped.set()
        except (OSError, asyncio.TimeoutError):
            pass
        finally:
            writer.close()

    control_path = state / "control.sock"
    if control_path.exists():
        if not control_path.is_socket() or control_path.stat().st_uid != os.getuid():
            raise RuntimeError("Refusing foreign relay control path")
        control_path.unlink()
    servers = [await asyncio.start_server(client, sock=item) for item in listeners]
    servers.append(await asyncio.start_unix_server(control, path=str(control_path), limit=1024))
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
        request = json.loads(sys.stdin.buffer.readline(8192))
        asyncio.run(serve(request["proxy_url"], sys.argv[1]))
    except (OSError, ValueError, RuntimeError, KeyError):
        print("Proxy relay failed closed. Check proxy settings and relay ownership.", file=sys.stderr)
        sys.exit(1)
