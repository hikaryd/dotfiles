import array
import base64
import ipaddress
import os
from pathlib import Path
import re
import socket
import sys
import time
from urllib.parse import unquote

sys.path.insert(0, str(Path(__file__).resolve().parent))
from guard import validate_proxy


TIMEOUT = 15
HEADER_LIMIT = 16384


def connect_tunnel(host, port, url):
    proxy = validate_proxy(url)
    if proxy.scheme != "http" or proxy.hostname != "127.0.0.1":
        raise ValueError("SSH requires the configured local HTTP relay")
    if not re.fullmatch(r"[0-9]{1,5}", str(port)) or not 1 <= int(port) <= 65535:
        raise ValueError("Invalid SSH port")
    port = int(port)
    if not isinstance(host, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,252}", host):
        try:
            if not isinstance(host, str) or "%" in host:
                raise ValueError
            host = "[" + str(ipaddress.IPv6Address(host.strip("[]"))) + "]"
        except ValueError:
            raise ValueError("Invalid SSH host") from None
    if host.lower().rstrip(".") == "github.com" and port == 22:
        host, port = "ssh.github.com", 443
    authority = "{}:{}".format(host, port)
    request = "CONNECT {} HTTP/1.1\r\nHost: {}\r\n".format(authority, authority)
    if proxy.username is not None:
        credentials = unquote(proxy.username) + ":" + unquote(proxy.password or "")
        request += "Proxy-Authorization: Basic " + base64.b64encode(credentials.encode()).decode() + "\r\n"
    deadline = time.monotonic() + TIMEOUT
    connection = socket.create_connection((proxy.hostname, proxy.port), timeout=TIMEOUT)
    try:
        connection.settimeout(max(0.001, deadline - time.monotonic()))
        connection.sendall((request + "\r\n").encode("ascii"))
        header = bytearray()
        # Do not consume an SSH banner delivered in the same packet as the HTTP headers.
        while not header.endswith(b"\r\n\r\n"):
            if len(header) >= HEADER_LIMIT:
                raise ValueError("Proxy response headers too large")
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError
            connection.settimeout(remaining)
            data = connection.recv(1)
            if not data:
                raise ValueError("Proxy closed before CONNECT completed")
            header.extend(data)
        status = re.fullmatch(rb"HTTP/1\.[01] ([0-9]{3})(?: [^\r\n]*)?", header.split(b"\r\n", 1)[0])
        if status is None:
            raise ValueError("Invalid proxy response")
        code = int(status.group(1))
        if not 200 <= code < 300:
            raise ValueError("Proxy rejected SSH CONNECT (HTTP {})".format(code))
        connection.settimeout(None)
        return connection
    except BaseException:
        connection.close()
        raise


def main():
    try:
        if len(sys.argv) != 3:
            raise ValueError("Expected SSH host and port")
        with connect_tunnel(sys.argv[1], sys.argv[2], os.environ.get("HTTPS_PROXY", "")) as connection:
            with socket.socket(fileno=os.dup(1)) as channel:
                channel.sendmsg([b"\0"], [(socket.SOL_SOCKET, socket.SCM_RIGHTS,
                                          array.array("i", [connection.fileno()]))])
    except (OSError, ValueError) as error:
        message = str(error) if isinstance(error, ValueError) else "Proxy connection or descriptor handoff failed"
        print("claude-ssh: " + message, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
