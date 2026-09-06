#!/usr/bin/env python3
"""Генератор конфига sing-box из VLESS-подписки.

Читает параметры из /etc/sing-box/vpn.env, тянет подписку, строит config.json
(tproxy-вход, selector+urltest, гео-split РФ->direct, adblock через DNS-reject,
clash-API панель). Запускается systemd-таймером для авто-обновления узлов.
"""
import json
import os
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
        for line in open(path, encoding="utf-8"):
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
    if os.path.exists(path) and os.path.getsize(path) > 0:
        if time.time() - os.path.getmtime(path) < 7 * 24 * 3600:
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
        return None  # sing-box не поддерживает xhttp
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


def main() -> None:
    if not SUB_URL:
        sys.exit("SUB_URL is empty in " + CONF_FILE)

    ensure_srs("geoip-ru")
    if ADBLOCK:
        ensure_srs("geosite-ads")

    nodes, seen = [], set()
    for i, line in enumerate(fetch(SUB_URL).splitlines()):
        line = line.strip()
        if not line.startswith("vless://"):
            continue
        try:
            ob = parse_vless(line, i)
        except Exception as e:
            print(f"skip line {i}: {e}", file=sys.stderr)
            continue
        if not ob:
            continue
        if ob["tag"] in seen:
            ob["tag"] += f"_{i}"
        seen.add(ob["tag"])
        nodes.append(ob)
    if not nodes:
        sys.exit("NO NODES PARSED — keeping old config")
    node_tags = [n["tag"] for n in nodes]
    secret = open(SECRET_FILE).read().strip()

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
            {"type": "tproxy", "tag": "tproxy-in", "listen": "0.0.0.0", "listen_port": TPROXY_PORT}
        ],
        "outbounds": (
            [
                {"type": "selector", "tag": "select", "outbounds": ["auto"] + node_tags, "default": "auto"},
                {"type": "urltest", "tag": "auto", "outbounds": node_tags,
                 "url": "https://www.gstatic.com/generate_204", "interval": "5m", "tolerance": 50},
            ]
            + nodes
            + [{"type": "direct", "tag": "direct"}]
        ),
        "route": {
            "rules": [
                {"action": "sniff"},
                {"protocol": "dns", "action": "hijack-dns"},
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
    r = subprocess.run(["sing-box", "check", "-c", tmp], capture_output=True, text=True)
    if r.returncode != 0:
        sys.exit("CONFIG CHECK FAILED:\n" + r.stdout + r.stderr)
    os.replace(tmp, CFG)
    print(f"OK: {len(nodes)} nodes, adblock={'on' if ADBLOCK else 'off'}")


if __name__ == "__main__":
    main()
