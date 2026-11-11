import fcntl
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import secrets
import string
import subprocess
import time
from urllib.parse import unquote

from guard import validate_proxy

LABEL = "org.dots.claude-guard"
OWNER = "browser-v1"
GUARD = "dots-claude-browser-net"
BROWSER = "dots-claude-browser"
BUILD = "dots-claude-browser-build"
IMAGE = "dots-claude-browser:local"
VOLUME = "dots-claude-browser-profile"
NETWORK = "dots-claude-browser-network"
DOCKER = ["/usr/local/bin/docker", "--host", "unix://" + str(Path.home() / ".docker/run/docker.sock")]

BOOTSTRAP = r'''
IFS= read -r http_proxy
export http_proxy https_proxy="$http_proxy" HTTP_PROXY="$http_proxy" HTTPS_PROXY="$http_proxy"
unset ALL_PROXY all_proxy NO_PROXY no_proxy
apk add --no-cache firefox xvfb x11vnc openbox font-dejavu dbus
addgroup -g 1000 browser
adduser -D -u 1000 -G browser -h /home/browser browser
mkdir -p /home/browser /profile /usr/lib/firefox/defaults/pref
chown 1000:1000 /home/browser /profile
unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY
'''


def docker(arguments, input_text=None, missing=False):
    result = subprocess.run(DOCKER + arguments, input=input_text,
                            capture_output=True, text=True, close_fds=True)
    if result.returncode:
        absent_network = arguments[:2] == ["network", "inspect"] and "not found" in result.stderr
        if missing and ("no such" in result.stderr.lower() or absent_network):
            return None
        raise RuntimeError("Docker operation failed. No direct-network fallback. Check Docker Desktop and cached images")
    return result.stdout.strip()


def owned(kind, name):
    raw = docker([kind, "inspect", name], missing=True)
    if raw is None:
        return None
    info = json.loads(raw)[0]
    labels = info.get("Labels", {}) if kind in ("volume", "network") else info.get("Config", {}).get("Labels", {})
    if (labels or {}).get(LABEL) != OWNER:
        raise RuntimeError("Refusing foreign Docker resource: " + name)
    return info


def options(config):
    url = config.get("browser_proxy_url") or config.get("proxy_url", "")
    proxy = validate_proxy(url)
    address = ipaddress.IPv4Address(proxy.hostname)
    if address.is_loopback or address.is_unspecified or address.is_multicast or address.is_link_local:
        raise ValueError("Browser proxy must be a Docker-reachable numeric IPv4, not loopback/link-local")
    memory = config.get("browser_memory_mb", 1024)
    cpus = config.get("browser_cpus", 2)
    port = config.get("browser_port", 5909)
    if type(memory) is not int or not 512 <= memory <= 8192:
        raise ValueError("browser_memory_mb must be between 512 and 8192")
    if type(cpus) not in (int, float) or not 0.25 <= cpus <= 8:
        raise ValueError("browser_cpus must be between 0.25 and 8")
    if type(port) is not int or not 1024 <= port <= 65535:
        raise ValueError("browser_port must be between 1024 and 65535")
    base = config.get("browser_base_image", "moby/buildkit:buildx-stable-1")
    if not isinstance(base, str) or not base or base.startswith("-") or any(c.isspace() for c in base):
        raise ValueError("Invalid browser_base_image")
    return url, proxy, memory, cpus, port, base


def cleanup():
    for name in (BROWSER, BUILD, GUARD):
        if owned("container", name):
            docker(["container", "rm", "-f", "-v", name])
    if owned("network", NETWORK):
        docker(["network", "rm", NETWORK])


def guard_ready():
    info = owned("container", GUARD)
    if not info or not info["State"]["Running"]:
        raise RuntimeError("Browser firewall is not running")
    host = info["HostConfig"]
    if not host.get("ReadonlyRootfs") or host.get("Privileged") or host.get("NetworkMode") != NETWORK:
        raise RuntimeError("Browser firewall security configuration changed")
    added = {value.removeprefix("CAP_") for value in (host.get("CapAdd") or [])}
    if added != {"NET_ADMIN"} or set(host.get("CapDrop") or []) != {"ALL"}:
        raise RuntimeError("Browser firewall capabilities changed")
    if "no-new-privileges" not in (host.get("SecurityOpt") or []):
        raise RuntimeError("Browser firewall lost no-new-privileges")
    bindings = host.get("PortBindings", {}).get("5900/tcp", [])
    if len(bindings) != 1 or bindings[0].get("HostIp") != "127.0.0.1":
        raise RuntimeError("VNC port is not loopback-only")
    command = info["Config"].get("Cmd", [])
    if len(command) != 4 or command[0] != "/guard.sh":
        raise RuntimeError("Unexpected firewall command")
    address, port, gateway = command[1:]
    validate_proxy("http://{}:{}".format(address, port))
    ipaddress.IPv4Address(gateway)
    networks = info["NetworkSettings"]["Networks"]
    if set(networks) != {NETWORK} or networks[NETWORK].get("Gateway") != gateway:
        raise RuntimeError("Unexpected browser network or VNC ingress gateway")
    rules = docker(["exec", GUARD, "iptables", "-S"])
    rules6 = docker(["exec", GUARD, "ip6tables", "-S"])
    policies = {"-P {} DROP".format(chain) for chain in ("INPUT", "OUTPUT", "FORWARD")}
    expected = policies | {
        "-A INPUT -s {}/32 -p tcp -m tcp --sport {} -m conntrack --ctstate ESTABLISHED -j ACCEPT".format(address, port),
        "-A INPUT -s {}/32 -p tcp -m tcp --dport 5900 -m conntrack --ctstate NEW,ESTABLISHED -j ACCEPT".format(gateway),
        "-A OUTPUT -d {}/32 -p tcp -m tcp --dport {} -j ACCEPT".format(address, port),
        "-A OUTPUT -d {}/32 -p tcp -m tcp --sport 5900 -m conntrack --ctstate ESTABLISHED -j ACCEPT".format(gateway),
    }
    if set(rules.splitlines()) != expected or set(rules6.splitlines()) != policies:
        raise RuntimeError("Firewall rules differ from the strict proxy-only policy")
    return info


def start_guard(root, proxy, port, base):
    image = json.loads(docker(["image", "inspect", base]))[0]["Id"]
    if owned("network", NETWORK):
        raise RuntimeError("Browser network already exists, run claude-browser stop first")
    docker(["network", "create", "--driver", "bridge", "--label", LABEL + "=" + OWNER, NETWORK])
    network = owned("network", NETWORK)
    gateway = network["IPAM"]["Config"][0]["Gateway"]
    ipaddress.IPv4Address(gateway)
    docker(["run", "-d", "--pull", "never", "--name", GUARD, "--label", LABEL + "=" + OWNER,
            "--network", NETWORK,
            "--entrypoint", "/bin/sh", "--cap-drop", "ALL", "--cap-add", "NET_ADMIN",
            "--security-opt", "no-new-privileges", "--read-only", "--log-driver", "none",
            "--memory", "64m", "--memory-swap", "64m", "--cpus", "0.25", "--pids-limit", "32",
            "--sysctl", "net.ipv6.conf.all.disable_ipv6=1", "--sysctl", "net.ipv6.conf.default.disable_ipv6=1",
            "--tmpfs", "/run:rw,nosuid,nodev,size=1m", "--publish", "127.0.0.1:{}:5900".format(port),
            "--mount", "type=bind,src={},dst=/guard.sh,readonly".format(root / "firewall.sh"),
            image, "/guard.sh", proxy.hostname, str(proxy.port), gateway])
    for _ in range(50):
        try:
            guard_ready()
            return image
        except RuntimeError:
            time.sleep(0.1)
    raise RuntimeError("Browser firewall failed to initialize")


def prepare(config, root, state):
    url, proxy, _, _, port, base = options(config)
    if owned("container", BROWSER) and owned("container", BROWSER)["State"]["Running"]:
        raise RuntimeError("Stop the browser before preparing an image")
    if docker(["image", "inspect", IMAGE], missing=True):
        owned("image", IMAGE)
    cleanup()
    try:
        image = start_guard(root, proxy, port, base)
        if not owned("volume", VOLUME):
            docker(["volume", "create", "--label", LABEL + "=" + OWNER, VOLUME])
        docker(["run", "-d", "--pull", "never", "--name", BUILD, "--label", LABEL + "=" + OWNER,
                "--network", "container:" + GUARD, "--cap-drop", "ALL", "--cap-add", "CHOWN",
                "--cap-add", "DAC_OVERRIDE", "--cap-add", "FOWNER", "--cap-add", "SETUID", "--cap-add", "SETGID",
                "--security-opt", "no-new-privileges", "--log-driver", "none", "--entrypoint", "/bin/sh",
                "--mount", "type=volume,src={},dst=/profile,volume-nocopy".format(VOLUME), image, "-c", "exec sleep infinity"])
        guard_ready()
        docker(["exec", "-i", BUILD, "/bin/sh", "-eu", "-c", BOOTSTRAP], input_text=url + "\n")
        docker(["stop", BUILD])
        docker(["commit", "--change", "LABEL {}={}".format(LABEL, OWNER), BUILD, IMAGE])
        print("Browser image prepared through the guarded proxy. No credentials stored in image ENV.")
    finally:
        cleanup()


def start(config, root, state):
    url, proxy, memory, cpus, port, base = options(config)
    digest = hashlib.sha256(url.encode()).hexdigest()
    record = state / "browser.json"
    existing = owned("container", BROWSER)
    if existing and existing["State"]["Running"]:
        guard = guard_ready()
        host = existing["HostConfig"]
        if (host.get("NetworkMode") != "container:" + guard["Id"] or host.get("Privileged")
                or not host.get("ReadonlyRootfs") or set(host.get("CapDrop") or []) != {"ALL"}
                or host.get("CapAdd") or existing["Config"].get("User") != "1000:1000"):
            raise RuntimeError("Running browser is no longer in the expected sandbox")
        saved = json.loads(record.read_text()) if record.exists() else {}
        if (saved.get("proxy") != digest or saved.get("port") != port or saved.get("automatic_auth") != 1
                or guard["HostConfig"]["PortBindings"]["5900/tcp"][0].get("HostPort") != str(port)
                or host.get("Memory") != memory * 1024 * 1024 or host.get("NanoCpus") != int(cpus * 1000000000)):
            raise RuntimeError("Browser configuration changed. Run claude-browser stop before restarting")
        print("Browser already running at vnc://127.0.0.1:{}".format(port))
        return
    if not owned("image", IMAGE):
        raise RuntimeError("Run claude-browser prepare after setting the proxy")
    if not owned("volume", VOLUME):
        raise RuntimeError("Missing browser profile volume. Run claude-browser prepare")
    cleanup()
    try:
        start_guard(root, proxy, port, base)
        password_file = state / "browser-vnc-password"
        if not password_file.exists():
            with open(password_file, "x") as output:
                output.write("".join(secrets.choice(string.ascii_letters + string.digits) for _ in range(8)) + "\n")
            password_file.chmod(0o600)
        if password_file.is_symlink() or password_file.stat().st_mode & 0o077:
            raise RuntimeError("VNC password must be a private regular file")
        password = password_file.read_text().strip()
        if len(password) != 8 or not password.isascii() or not password.isalnum():
            raise RuntimeError("Invalid VNC password file")
        docker(["create", "--pull", "never", "--name", BROWSER, "--label", LABEL + "=" + OWNER,
                "--network", "container:" + GUARD, "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
                "--read-only", "--user", "1000:1000", "--init", "--log-driver", "none", "--pids-limit", "256",
                "--memory", str(memory) + "m", "--memory-swap", str(memory) + "m", "--cpus", str(cpus),
                "--shm-size", "256m", "--tmpfs", "/tmp:rw,nosuid,nodev,size=256m",
                "--tmpfs", "/run:rw,nosuid,nodev,size=16m,mode=1777", "-i", "--entrypoint", "/bin/sh",
                "--env", "HOME=/profile", "--env", "GUARD_PROXY_HOST=" + proxy.hostname,
                "--env", "GUARD_PROXY_PORT=" + str(proxy.port), "--env", "GUARD_PROXY_TLS=" + str(int(proxy.scheme == "https")),
                "--mount", "type=volume,src={},dst=/profile,volume-nocopy".format(VOLUME),
                "--mount", "type=bind,src={},dst=/browser-start.sh,readonly".format(root / "browser-start.sh"),
                "--mount", "type=bind,src={},dst=/usr/lib/firefox/defaults/pref/guard.js,readonly".format(root / "firefox-prefs.js"),
                "--mount", "type=bind,src={},dst=/usr/lib/firefox/mozilla.cfg,readonly".format(root / "firefox.cfg"),
                IMAGE, "/browser-start.sh"])
        process = subprocess.Popen(DOCKER + ["start", "-ai", BROWSER], stdin=subprocess.PIPE,
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True, close_fds=True)
        authentication = json.dumps({"username": unquote(proxy.username or ""),
                                     "password": unquote(proxy.password or "")}, ensure_ascii=True)
        process.stdin.write((password + "\n" + authentication + "\n").encode())
        process.stdin.close()
        for _ in range(100):
            if process.poll() is not None:
                raise RuntimeError("Browser startup failed, no unguarded retry")
            try:
                docker(["exec", BROWSER, "test", "-f", "/tmp/browser-ready"])
                break
            except RuntimeError:
                time.sleep(0.1)
        else:
            raise RuntimeError("Browser readiness timed out")
        record.write_text(json.dumps({"proxy": digest, "port": port, "automatic_auth": 1}) + "\n")
        record.chmod(0o600)
        print("Connect Screen Sharing to vnc://127.0.0.1:{}".format(port))
        print("VNC password file: " + str(password_file))
        print("Proxy authentication is supplied automatically from private settings.")
    except BaseException:
        cleanup()
        raise


def main(config, root, state, arguments):
    if not arguments or arguments[0] not in ("prepare", "start", "stop", "status") or len(arguments) != 1:
        raise ValueError("Usage: claude-browser prepare|start|stop|status")
    action = arguments[0]
    if action in ("prepare", "start"):
        options(config)
    with open(state / "browser.lock", "a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        if action == "prepare":
            prepare(config, root, state)
        elif action == "start":
            start(config, root, state)
        elif action == "stop":
            cleanup()
            print("Browser and firewall stopped. Persistent profile retained.")
        else:
            info = owned("container", BROWSER)
            print("Browser running" if info and info["State"]["Running"] else "Browser stopped")
