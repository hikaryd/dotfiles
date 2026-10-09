#!/usr/bin/env python3
"""Генератор конфига sing-box из VLESS/Hysteria2-подписки.

Читает параметры из /etc/sing-box/vpn.env, тянет подписку, строит config.json
(tproxy-вход, selector+urltest, гео-split РФ->direct, adblock через DNS-reject,
clash-API панель). Запускается systemd-таймером для авто-обновления узлов.
"""
import json
import os
import re
import ssl
import subprocess
import sys
import time
import urllib.parse
import urllib.request

CONF_FILE = os.environ.get("VPN_ENV", "/etc/sing-box/vpn.env")
ETC = "/etc/sing-box"
CFG = f"{ETC}/config.json"
SECRET_FILE = f"{ETC}/secret.txt"

UTLS_OK = {"chrome", "firefox", "edge", "safari", "360", "qq", "ios",
           "android", "random", "randomized"}

RULESETS = {
    "geoip-ru": "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-ru.srs",
    "geosite-ads": "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-category-ads-all.srs",
}

# Эти домены идут через подписку независимо от GeoIP и DNS-adblock.
PROXY_DOMAINS = ["aquadx.hydev.org"]


def load_conf(path: str) -> dict:
    conf = {}
    if os.path.exists(path):
        with open(path, encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("#") or "=" not in line:
                    continue
                k, v = line.split("=", 1)
                conf[k.strip()] = v.strip().strip('"').strip("'")
    return conf


CONF = load_conf(CONF_FILE)
SUB_URL = CONF.get("SUB_URL", "")
TPROXY_PORT = int(CONF.get("TPROXY_PORT", "7895"))
CLASH_PORT = int(CONF.get("CLASH_PORT", "9090"))
HEALTH_PORT = int(CONF.get("HEALTH_PORT", "7896"))
AUTO_INCLUDE = CONF.get("AUTO_INCLUDE", "")
AUTO_EXCLUDE = CONF.get("AUTO_EXCLUDE", "LTE|4G|3G|Обход БС|⬇|⬆")
AUTO_PREFER = CONF.get("AUTO_PREFER", "")
AUTO_MAX = int(CONF.get("AUTO_MAX", "8"))
ADBLOCK = CONF.get("ADBLOCK", "1") == "1"
ADBLOCK_WHITELIST = [d.strip() for d in CONF.get(
    "ADBLOCK_WHITELIST", "tmdb.org,themoviedb.org,b-cdn.net").split(",") if d.strip()]


def srs_path(tag: str) -> str:
    return f"{ETC}/{tag}.srs"


def fetch(url: str) -> str:
    ctx = ssl.create_default_context()
    req = urllib.request.Request(url, headers={"User-Agent": "sing-box"})
    return urllib.request.urlopen(req, timeout=30, context=ctx).read().decode("utf-8", "ignore")


def ensure_srs(tag: str) -> None:
    url, path = RULESETS[tag], srs_path(tag)
    if (os.path.exists(path) and os.path.getsize(path) > 0
            and time.time() - os.path.getmtime(path) < 7 * 24 * 3600):
        return
    try:
        ctx = ssl.create_default_context()
        req = urllib.request.Request(url, headers={"User-Agent": "sing-box"})
        blob = urllib.request.urlopen(req, timeout=30, context=ctx).read()
        with open(path, "wb") as f:
            f.write(blob)
        print(f"{tag}.srs updated ({len(blob)} bytes)")
    except Exception as e:
        print(f"{tag} download failed: {e}", file=sys.stderr)
        if not os.path.exists(path):
            raise


def parse_vless(line: str, idx: int):
    rest = line[len("vless://"):]
    frag = ""
    if "#" in rest:
        rest, frag = rest.split("#", 1)
        frag = urllib.parse.unquote(frag)
    userinfo, hostpart = rest.split("@", 1)
    uuid = userinfo
    hp, q = (hostpart.split("?", 1) + [""])[:2]
    host, port = hp.rsplit(":", 1)
    host, port = host.strip("[]"), int(port)
    p = dict(urllib.parse.parse_qsl(q))
    typ = p.get("type", "tcp")
    if typ == "xhttp":
        raise NodeParseError("xhttp transport is not supported by sing-box")
    if typ not in ("tcp", "grpc", "http"):
        raise NodeParseError("unsupported VLESS transport")
    if not uuid or not host or not 1 <= port <= 65535:
        raise NodeParseError("missing or invalid VLESS endpoint/authentication")
    fp = p.get("fp", "chrome")
    if fp not in UTLS_OK:
        fp = "chrome"
    name = frag.split(" - ")[0].strip() if frag else f"node{idx}"
    ob = {
        "type": "vless", "tag": f"{name} #{idx}",
        "server": host, "server_port": port, "uuid": uuid,
        "tls": {
            "enabled": True, "server_name": p.get("sni", ""),
            "utls": {"enabled": True, "fingerprint": fp},
            "reality": {"enabled": True, "public_key": p.get("pbk", ""), "short_id": p.get("sid", "")},
        },
    }
    if p.get("flow"):
        ob["flow"] = p["flow"]
    if p.get("insecure") in ("1", "true"):
        ob["tls"]["insecure"] = True
    if typ == "grpc":
        ob["transport"] = {"type": "grpc", "service_name": p.get("serviceName", "")}
    elif typ == "http":
        ob["transport"] = {"type": "http", "host": [p.get("host", p.get("sni", ""))]}
    return ob


class NodeParseError(ValueError):
    """Parser failure with a fixed reason safe to print without URI credentials."""


def parse_hysteria2(line: str, idx: int):
    try:
        uri = urllib.parse.urlsplit(line)
        auth, separator, address = uri.netloc.rpartition("@")
        password = urllib.parse.unquote(auth, errors="strict")
        if not separator or not password:
            raise NodeParseError("missing Hysteria2 authentication")
        if not uri.hostname:
            raise NodeParseError("missing Hysteria2 server")
        if uri.path not in ("", "/"):
            raise NodeParseError("unsupported Hysteria2 URI path")
        params = dict(urllib.parse.parse_qsl(uri.query, keep_blank_values=True))
        if any(key in params for key in ("pinSHA256", "ech", "mport")):
            raise NodeParseError("unsupported Hysteria2 URI parameter")
        insecure = params.get("insecure", "0").lower()
        if insecure not in ("0", "1", "false", "true"):
            raise NodeParseError("invalid Hysteria2 insecure flag")

        # Hysteria's multi-port address uses '-', sing-box ranges use ':'.
        port_text = address.split("]", 1)[1] if address.startswith("[") else address.partition(":")[2]
        if address.startswith("["):
            if port_text and not port_text.startswith(":"):
                raise NodeParseError("invalid Hysteria2 server port")
            port_text = port_text.removeprefix(":")
        port_text = port_text or "443"
        ports = []
        for part in port_text.split(","):
            if not re.fullmatch(r"[0-9]+(?:-[0-9]+)?", part):
                raise NodeParseError("invalid Hysteria2 server port")
            start, _, end = part.partition("-")
            if not 1 <= int(start) <= int(end or start) <= 65535:
                raise NodeParseError("invalid Hysteria2 server port")
            ports.append(part.replace("-", ":"))
        name = urllib.parse.unquote(uri.fragment).split(" - ")[0].strip() if uri.fragment else f"node{idx}"
        outbound = {
            "type": "hysteria2", "tag": f"{name} #{idx}",
            "server": uri.hostname, "password": password,
            "tls": {"enabled": True, "server_name": params.get("sni", "")},
        }
        if len(ports) == 1 and ":" not in ports[0]:
            outbound["server_port"] = int(ports[0])
        else:
            outbound["server_ports"] = ports
        if insecure in ("1", "true"):
            outbound["tls"]["insecure"] = True
        if params.get("obfs"):
            if params["obfs"] != "salamander":
                raise NodeParseError("unsupported Hysteria2 obfuscation")
            if not params.get("obfs-password"):
                raise NodeParseError("missing Hysteria2 obfuscation password")
            outbound["obfs"] = {"type": "salamander", "password": params["obfs-password"]}
        elif params.get("obfs-password"):
            raise NodeParseError("missing Hysteria2 obfuscation type")
        return outbound
    except NodeParseError:
        raise
    except ValueError:
        raise NodeParseError("malformed Hysteria2 URI") from None


def parse_node(line: str, idx: int):
    scheme = line.partition("://")[0].lower()
    parser = {"vless": parse_vless, "hysteria2": parse_hysteria2, "hy2": parse_hysteria2}.get(scheme)
    if parser is None:
        raise NodeParseError("unsupported protocol (supported: vless, hysteria2, hy2)")
    try:
        return parser(line, idx)
    except NodeParseError:
        raise
    except (ValueError, KeyError):
        raise NodeParseError("malformed node URI") from None


def parse_subscription(source: str):
    nodes, seen = [], set()
    for idx, line in enumerate(source.splitlines()):
        line = line.strip()
        if not line:
            continue
        try:
            node = parse_node(line, idx)
        except NodeParseError as exc:
            print(f"skip line {idx}: {exc}", file=sys.stderr)
            continue
        if node["tag"] in seen:
            node["tag"] += f"_{idx}"
        seen.add(node["tag"])
        nodes.append(node)
    return nodes


def auto_pool(nodes, include, exclude, limit, prefer=""):
    """Keep manual nodes intact; prioritize matching auto aliases before the cap."""
    if limit < 1:
        raise ValueError("AUTO_MAX must be positive")
    try:
        include_re = re.compile(include, re.IGNORECASE) if include else None
        exclude_re = re.compile(exclude, re.IGNORECASE) if exclude else None
        prefer_re = re.compile(prefer, re.IGNORECASE) if prefer else None
    except re.error as exc:
        raise ValueError("Invalid AUTO_INCLUDE/AUTO_EXCLUDE/AUTO_PREFER regex") from exc
    candidates = []
    for node in nodes:
        tag = node["tag"]
        if include_re and not include_re.search(tag):
            continue
        if exclude_re and exclude_re.search(tag):
            continue
        candidates.append(node)
    if prefer_re:
        candidates.sort(key=lambda node: not prefer_re.search(node["tag"]))
    tags, seen = [], set()
    for node in candidates:
        identity = json.dumps({k: v for k, v in node.items() if k != "tag"}, sort_keys=True)
        if identity in seen:
            continue
        seen.add(identity)
        tags.append(node["tag"])
        if len(tags) == limit:
            break
    if not tags:
        raise ValueError("Auto pool is empty; keeping old config. Check AUTO_INCLUDE/AUTO_EXCLUDE")
    return tags


def main() -> None:
    os.umask(0o077)
    if not SUB_URL:
        sys.exit("SUB_URL is empty in " + CONF_FILE)

    ensure_srs("geoip-ru")
    if ADBLOCK:
        ensure_srs("geosite-ads")

    nodes = parse_subscription(fetch(SUB_URL))
    if not nodes:
        sys.exit("NO NODES PARSED — keeping old config")
    node_tags = [n["tag"] for n in nodes]
    auto_tags = auto_pool(nodes, AUTO_INCLUDE, AUTO_EXCLUDE, AUTO_MAX, AUTO_PREFER)
    with open(SECRET_FILE) as f:
        secret = f.read().strip()
    if not secret:
        sys.exit("Panel secret must not be empty")
    if len({TPROXY_PORT, CLASH_PORT, HEALTH_PORT}) != 3:
        sys.exit("TPROXY_PORT, CLASH_PORT and HEALTH_PORT must differ")

    dns_rules = [{"domain": PROXY_DOMAINS, "server": "proxy-dns"}]
    if ADBLOCK:
        if ADBLOCK_WHITELIST:
            dns_rules.append({"domain_suffix": ADBLOCK_WHITELIST, "server": "proxy-dns"})
        dns_rules.append({"rule_set": "geosite-ads", "action": "reject"})
    dns_rules += [
        {"clash_mode": "direct", "server": "direct-dns"},
        {"clash_mode": "global", "server": "proxy-dns"},
    ]

    rule_sets = [{"type": "local", "tag": "geoip-ru", "format": "binary", "path": srs_path("geoip-ru")}]
    if ADBLOCK:
        rule_sets.append({"type": "local", "tag": "geosite-ads", "format": "binary", "path": srs_path("geosite-ads")})

    config = {
        "log": {"level": "warn", "timestamp": True},
        "dns": {
            "servers": [
                {"tag": "proxy-dns", "type": "https", "server": "1.1.1.1", "detour": "select"},
                {"tag": "direct-dns", "type": "local", "detour": "direct"},
            ],
            "rules": dns_rules,
            "final": "proxy-dns",
            "strategy": "prefer_ipv4",
        },
        "inbounds": [
            {"type": "tproxy", "tag": "tproxy-in", "listen": "0.0.0.0", "listen_port": TPROXY_PORT},
            {"type": "mixed", "tag": "health-in", "listen": "127.0.0.1", "listen_port": HEALTH_PORT}
        ],
        "outbounds": (
            [
                {"type": "selector", "tag": "select", "outbounds": ["auto"] + node_tags, "default": "auto"},
                {"type": "urltest", "tag": "auto", "outbounds": auto_tags,
                 "url": "https://www.gstatic.com/generate_204", "interval": "5m", "tolerance": 50},
            ]
            + nodes
            + [{"type": "direct", "tag": "direct"}]
        ),
        "route": {
            "rules": [
                {"action": "sniff"},
                {"protocol": "dns", "action": "hijack-dns"},
                {"inbound": ["health-in"], "action": "route", "outbound": "select"},
                {"domain": PROXY_DOMAINS, "action": "route", "outbound": "select"},
                {"ip_is_private": True, "action": "route", "outbound": "direct"},
                {"rule_set": "geoip-ru", "action": "route", "outbound": "direct"},
            ],
            "rule_set": rule_sets,
            "final": "select",
            "auto_detect_interface": True,
            "default_domain_resolver": {"server": "direct-dns"},
        },
        "experimental": {
            "clash_api": {
                "external_controller": f"0.0.0.0:{CLASH_PORT}",
                "secret": secret,
                "external_ui": f"{ETC}/ui",
                "default_mode": "rule",
            },
            "cache_file": {"enabled": True, "path": "/var/lib/sing-box/cache.db"},
        },
    }

    tmp = CFG + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(config, f, ensure_ascii=False, indent=2)
    r = subprocess.run(["sing-box", "check", "-c", tmp], capture_output=True, text=True, check=False)
    if r.returncode != 0:
        sys.exit(f"CONFIG CHECK FAILED (exit {r.returncode}) — keeping old config")
    os.replace(tmp, CFG)
    print(f"OK: {len(nodes)} manual nodes, {len(auto_tags)} auto nodes, adblock={'on' if ADBLOCK else 'off'}")


if __name__ == "__main__":
    main()
