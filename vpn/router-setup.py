#!/usr/bin/env python3
"""Настройка роутера Keenetic через HTTP RCI.

Режимы (env ACTION, по умолчанию install):
  install    — AmneziaWG-клиент (WireGuard + asc-обфускация), маршрут по
               умолчанию, ping-check (failover на WAN), DNS в туннель.
  identify   — печатает NAME= (hostname/модель) и CURPUB= (текущий WG-ключ)
               для реестра пиров на VPS.

Все параметры берутся из переменных окружения (их задаёт install.sh).
"""
import hashlib
import http.cookiejar
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request

H = os.environ
BASE = f"http://{H.get('ROUTER_HOST', '192.168.254.1')}"
LOGIN = H.get("ROUTER_USER", "admin")
PASS = H["ROUTER_PASS"]
LAN_IP = H.get("ROUTER_LAN_IP", H.get("ROUTER_HOST", "192.168.254.1"))
IFACE = H.get("WG_IFACE", "Wireguard0")
DHCP_POOL = H.get("DHCP_POOL", "_WEBADMIN")
ACTION = H.get("ACTION", "install")

cj = http.cookiejar.CookieJar()
op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))


def raw(path, data=None, method=None, hdr=None):
    return op.open(urllib.request.Request(BASE + path, data=data, headers=hdr or {}, method=method), timeout=25)


def auth():
    try:
        raw("/auth")
        return
    except urllib.error.HTTPError as e:
        realm = e.headers.get("X-NDM-Realm")
        ch = e.headers.get("X-NDM-Challenge")
    md5 = hashlib.md5(f"{LOGIN}:{realm}:{PASS}".encode()).hexdigest()
    sha = hashlib.sha256((ch + md5).encode()).hexdigest()
    raw("/auth", json.dumps({"login": LOGIN, "password": sha}).encode(),
        "POST", {"Content-Type": "application/json"})


def batch(cmds):
    r = raw("/rci/", json.dumps([{"parse": c} for c in cmds]).encode(),
            "POST", {"Content-Type": "application/json"})
    out = json.loads(r.read().decode())
    res = []
    for c, o in zip(cmds, out):
        st = o.get("parse", {}).get("status", [{}])
        msg = "; ".join(s.get("message", "") or s.get("status", "") for s in st)
        res.append((c, msg))
    return res


def run(cmds, label):
    print(f"\n[ {label} ]")
    for c, m in batch(cmds):
        print(f"  · {c[:60]:60} | {m[:90]}")


def get(path):
    return json.loads(raw(path).read().decode())


def identify():
    """Стабильное имя роутера + его текущий WG-ключ (для реестра пиров на VPS)."""
    auth()
    name = ""
    try:
        name = str(get("/rci/show/system").get("hostname") or "")
    except Exception:
        pass
    if not name:
        try:
            ver = get("/rci/show/version")
            name = str(ver.get("device") or ver.get("model") or "")
        except Exception:
            pass
    if not name:
        name = "router-" + H.get("ROUTER_HOST", "unknown")
    slug = re.sub(r"[^A-Za-z0-9_-]+", "-", name).strip("-") or "router"
    curpub = ""
    try:
        si = get(f"/rci/show/interface/{IFACE}")
        curpub = str((si.get("wireguard") or {}).get("public-key") or "")
    except Exception:
        pass
    print(f"NAME={slug}")
    print(f"CURPUB={curpub}")


def install():
    server_host = H["SERVER_HOST"]
    awg_port = H.get("AWG_PORT", "51820")
    tun_cli = H.get("TUN_CLI", "10.8.2.2")
    tun_srv = H.get("TUN_SRV", "10.8.2.1")
    tun_mask = H.get("TUN_MASK", "255.255.255.0")
    mtu = H.get("MTU", "1280")
    server_pub = H["SERVER_PUB"]
    router_key = H["ROUTER_KEY"]
    asc = H["AWG_PARAMS"]  # "Jc Jmin Jmax S1 S2 H1 H2 H3 H4"
    priority = H.get("PRIORITY", "1000")

    auth()
    print(f"Авторизация на {BASE} — ок")

    # 1) интерфейс + ключ + AWG-обфускация
    run([
        f"interface {IFACE}",
        f"interface {IFACE} description AWG-VPN",
        f"interface {IFACE} wireguard asc {asc}",
        f"interface {IFACE} security-level public",
        f"interface {IFACE} ip address {tun_cli} {tun_mask}",
        f"interface {IFACE} ip mtu {mtu}",
        f"interface {IFACE} wireguard private-key {router_key}",
    ], "Интерфейс + ключ + AWG")

    # 1.5) убрать чужих пиров (напр. от прежнего сервера при миграции):
    # два пира с allow-ips 0.0.0.0/0 ломают cryptokey-routing и рассинхронят сессию
    try:
        si = get(f"/rci/show/interface/{IFACE}")
        stale = [pe.get("public-key") for pe in si.get("wireguard", {}).get("peer", [])
                 if pe.get("public-key") and pe.get("public-key") != server_pub]
        if stale:
            run([f"interface {IFACE} no wireguard peer {pk}" for pk in stale],
                "Удаление чужих пиров")
    except Exception as e:
        print("peer cleanup skipped:", e)

    # 2) пир (контекст сохраняется в одном POST)
    run([
        f"interface {IFACE}",
        f"wireguard peer {server_pub}",
        f"endpoint {server_host}:{awg_port}",
        "keepalive-interval 25",
        "allow-ips 0.0.0.0/0",
        "exit", "exit",
        f"interface {IFACE} up",
    ], "Пир + endpoint + поднятие")

    # 3) маршрут по умолчанию через туннель
    run([f"interface {IFACE} ip global {priority}"], "Default route через WG")

    # 4) ping-check (failover на WAN)
    run([
        "ping-check profile WGCHECK",
        f"host {tun_srv}",
        "update-interval 5",
        "max-fails 3",
        "timeout 3",
        "exit",
        f"interface {IFACE} ping-check profile WGCHECK",
    ], "Failover (ping-check)")

    # 5) DNS в туннель (анти-отравление РФ): чистые резолверы + убрать WAN-DNS/маршрут
    dns_cmds = [
        "no ip name-server 8.8.8.8",
        "no ip name-server 77.88.8.8",
        "ip name-server 1.1.1.1",
        "ip name-server 9.9.9.9",
        f"no ip dhcp pool {DHCP_POOL} dns-server",
        f"ip dhcp pool {DHCP_POOL} dns-server {LAN_IP}",
    ]
    run(dns_cmds, "DNS через туннель + DHCP")
    # убрать залипший host-маршрут к 8.8.8.8/77.88.8.8 через WAN (если есть)
    try:
        for r in get("/rci/show/ip/route"):
            d = str(r.get("destination", ""))
            if d.startswith(("8.8.8.8", "77.88.8.8")):
                run([f"no ip route {d} {r.get('interface', '')}"], f"Удаление WAN-маршрута {d}")
    except Exception as e:
        print("route cleanup skipped:", e)

    # 6) сохранить
    run(["system configuration save"], "Сохранение конфигурации")

    # статус
    time.sleep(5)
    si = get(f"/rci/show/interface/{IFACE}")
    pe = (si.get("wireguard", {}).get("peer") or [{}])[0]
    print(f"\nИтог: link={si.get('link')} connected={si.get('connected')} "
          f"global={si.get('global')} | peer online={pe.get('online')} "
          f"handshake={pe.get('last-handshake')}s")


def main():
    actions = {"install": install, "identify": identify}
    if ACTION not in actions:
        print(f"неизвестный ACTION={ACTION} (install|identify)", file=sys.stderr)
        sys.exit(2)
    actions[ACTION]()


if __name__ == "__main__":
    main()
