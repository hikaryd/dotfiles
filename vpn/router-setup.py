#!/usr/bin/env python3
"""Настройка роутера Keenetic через HTTP RCI.

Создаёт AmneziaWG-клиент (WireGuard + asc-обфускация), делает его маршрутом по
умолчанию, поднимает ping-check (failover на WAN), заворачивает DNS в туннель.
Все параметры берутся из переменных окружения (их задаёт install.sh).
"""
import hashlib
import http.cookiejar
import json
import os
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
SERVER_HOST = H["SERVER_HOST"]
AWG_PORT = H.get("AWG_PORT", "51820")
TUN_CLI = H.get("TUN_CLI", "10.8.2.2")
TUN_SRV = H.get("TUN_SRV", "10.8.2.1")
TUN_MASK = H.get("TUN_MASK", "255.255.255.0")
MTU = H.get("MTU", "1420")
SERVER_PUB = H["SERVER_PUB"]
ROUTER_KEY = H["ROUTER_KEY"]
ASC = H["AWG_PARAMS"]  # "Jc Jmin Jmax S1 S2 H1 H2 H3 H4"
PRIORITY = H.get("PRIORITY", "1000")
DHCP_POOL = H.get("DHCP_POOL", "_WEBADMIN")

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


def main():
    auth()
    print(f"Авторизация на {BASE} — ок")

    # 1) интерфейс + ключ + AWG-обфускация
    run([
        f"interface {IFACE}",
        f"interface {IFACE} description AWG-VPN",
        f"interface {IFACE} wireguard asc {ASC}",
        f"interface {IFACE} security-level public",
        f"interface {IFACE} ip address {TUN_CLI} {TUN_MASK}",
        f"interface {IFACE} ip mtu {MTU}",
        f"interface {IFACE} wireguard private-key {ROUTER_KEY}",
    ], "Интерфейс + ключ + AWG")

    # 2) пир (контекст сохраняется в одном POST)
    run([
        f"interface {IFACE}",
        f"wireguard peer {SERVER_PUB}",
        f"endpoint {SERVER_HOST}:{AWG_PORT}",
        "keepalive-interval 25",
        "allow-ips 0.0.0.0/0",
        "exit", "exit",
        f"interface {IFACE} up",
    ], "Пир + endpoint + поднятие")

    # 3) маршрут по умолчанию через туннель
    run([f"interface {IFACE} ip global {PRIORITY}"], "Default route через WG")

    # 4) ping-check (failover на WAN)
    run([
        "ping-check profile WGCHECK",
        f"host {TUN_SRV}",
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


if __name__ == "__main__":
    main()
