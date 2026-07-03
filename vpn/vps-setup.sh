#!/usr/bin/env bash
# shellcheck disable=SC2154  # Jc/Jmin/Jmax/… приходят из awg_params.env
# Серверная часть: AmneziaWG-сервер + sing-box (tproxy gateway) + adblock + панель.
# Запускается НА VPS (Debian/Ubuntu) скриптом install.sh. Идемпотентен.
# Параметры берутся из переменных окружения (см. дефолты ниже).
# В конце печатает блок ===VARS=== с данными для настройки роутера.
set -euo pipefail

SUB_URL=${SUB_URL:?need SUB_URL}
AWG_PORT=${AWG_PORT:-51820}
TUN_CIDR=${TUN_CIDR:-10.8.2.0/24}
SRV_CIDR=${SRV_CIDR:-10.8.2.1/24}
MTU=${MTU:-1280}
ADBLOCK=${ADBLOCK:-1}
TPROXY_PORT=${TPROXY_PORT:-7895}
CLASH_PORT=${CLASH_PORT:-9090}
ADBLOCK_WHITELIST=${ADBLOCK_WHITELIST:-tmdb.org,themoviedb.org,b-cdn.net}
GEN_SRC=${GEN_SRC:-/tmp/awg-generate.py}
PEERREG_SRC=${PEERREG_SRC:-/tmp/awg-peer-register.sh}
WORK=/root/awgvpn

say() { printf '   \033[36m·\033[0m %s\n' "$*"; }
export DEBIAN_FRONTEND=noninteractive

say "apt: пакеты"
apt-get update -qq
apt-get install -y -qq wireguard-tools golang git build-essential nftables curl jq unzip >/dev/null

mkdir -p "$WORK"; cd "$WORK"; umask 077
say "ключи + AWG-параметры"
[ -f server.key ] || { wg genkey >server.key; wg pubkey <server.key >server.pub; }
# ключи роутеров живут в реестре peers/ (peer-register.sh), по одному на роутер
if [ ! -f awg_params.env ]; then
  python3 - <<'PY'
import random
H=set()
while len(H)<4: H.add(random.randint(5, 2147483647))
H=list(H)
S1=random.randint(15,150); S2=random.randint(15,150)
while S1+56==S2: S2=random.randint(15,150)
Jc=random.randint(3,8); Jmin=random.randint(8,60); Jmax=random.randint(Jmin+10,120)
open("awg_params.env","w").write(
    f"Jc={Jc}\nJmin={Jmin}\nJmax={Jmax}\nS1={S1}\nS2={S2}\n"
    f"H1={H[0]}\nH2={H[1]}\nH3={H[2]}\nH4={H[3]}\n")
PY
fi
# shellcheck disable=SC1091
. ./awg_params.env

# параметры туннеля — нужны peer-register.sh для пересборки awg0.conf
cat >server.env <<EOF
SRV_CIDR=$SRV_CIDR
AWG_PORT=$AWG_PORT
MTU=$MTU
TUN_CIDR=$TUN_CIDR
EOF
install -m 0755 "$PEERREG_SRC" "$WORK/peer-register.sh"

say "Go (amneziawg-go требует >= 1.24; apt на старых ОС, напр. Debian 12, даёт 1.19)"
go_ok=0
if command -v go >/dev/null; then
  gv=$(go version 2>/dev/null | grep -oE "go[0-9]+\.[0-9]+" | head -1 | tr -d "go")
  gmaj=${gv%%.*}; gmin=${gv##*.}
  if [ "${gmaj:-0}" -gt 1 ] 2>/dev/null || { [ "${gmaj:-0}" -eq 1 ] 2>/dev/null && [ "${gmin:-0}" -ge 24 ] 2>/dev/null; }; then
    go_ok=1
  fi
fi
if [ "$go_ok" -ne 1 ]; then
  say "системный Go устарел/отсутствует — ставлю свежий из go.dev"
  garch=amd64; case "$(uname -m)" in aarch64|arm64) garch=arm64;; esac
  gver=$(curl -fsSL "https://go.dev/VERSION?m=text" | head -1)
  curl -fsSL "https://go.dev/dl/${gver}.linux-${garch}.tar.gz" -o /tmp/go.tgz
  rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz
  export PATH=/usr/local/go/bin:$PATH
fi

say "сборка amneziawg-go + amneziawg-tools"
if ! command -v amneziawg-go >/dev/null; then
  cd /opt && rm -rf amneziawg-go
  git clone --depth=1 https://github.com/amnezia-vpn/amneziawg-go >/dev/null 2>&1
  cd amneziawg-go
  make >/tmp/awg-go-build.log 2>&1 || { echo "ОШИБКА сборки amneziawg-go:" >&2; tail -15 /tmp/awg-go-build.log >&2; exit 1; }
  [ -x amneziawg-go ] || { echo "ОШИБКА: бинарник amneziawg-go не создан" >&2; exit 1; }
  cp amneziawg-go /usr/local/bin/
  cd "$WORK"
fi
if ! command -v awg-quick >/dev/null; then
  cd /opt && rm -rf amneziawg-tools
  git clone --depth=1 https://github.com/amnezia-vpn/amneziawg-tools >/dev/null 2>&1
  cd amneziawg-tools/src
  { make && make install; } >/tmp/awg-tools-build.log 2>&1 || { echo "ОШИБКА сборки amneziawg-tools:" >&2; tail -15 /tmp/awg-tools-build.log >&2; exit 1; }
  command -v awg-quick >/dev/null || { echo "ОШИБКА: awg-quick не установлен" >&2; exit 1; }
  cd "$WORK"
fi

say "установка sing-box"
command -v sing-box >/dev/null || curl -fsSL https://sing-box.app/install.sh | sh >/dev/null 2>&1

say "конфиг sing-box + панель"
mkdir -p /etc/sing-box/ui /var/lib/sing-box
[ -f /etc/sing-box/secret.txt ] || { head -c 24 /dev/urandom | base64 | tr -d '/+=' >/etc/sing-box/secret.txt; }
cat >/etc/sing-box/vpn.env <<EOF
SUB_URL=$SUB_URL
ADBLOCK=$ADBLOCK
TPROXY_PORT=$TPROXY_PORT
CLASH_PORT=$CLASH_PORT
ADBLOCK_WHITELIST=$ADBLOCK_WHITELIST
EOF
install -m 0755 "$GEN_SRC" /etc/sing-box/generate.py
if [ ! -f /etc/sing-box/ui/index.html ]; then
  curl -fsSL -o /tmp/zash.zip https://github.com/Zephyruso/zashboard/releases/latest/download/dist.zip
  rm -rf /tmp/zashx && mkdir -p /tmp/zashx && unzip -q /tmp/zash.zip -d /tmp/zashx
  if [ -d /tmp/zashx/dist ]; then cp -r /tmp/zashx/dist/* /etc/sing-box/ui/; else cp -r /tmp/zashx/* /etc/sing-box/ui/; fi
fi
mkdir -p /etc/systemd/system/sing-box.service.d
cat >/etc/systemd/system/sing-box.service.d/override.conf <<EOF
[Service]
User=root
Group=root
Restart=always
RestartSec=3
EOF

say "sysctl + nftables (tproxy)"
cat >/etc/sysctl.d/99-awggw.conf <<EOF
net.ipv4.ip_forward = 1
net.ipv4.conf.all.route_localnet = 1
EOF
sysctl -p /etc/sysctl.d/99-awggw.conf >/dev/null
# systemd-networkd по умолчанию (ManageForeignRoutingPolicyRules=yes) удаляет «чужие»
# ip rule при переконфигурации сети. Наше fwmark-правило (tproxy → table 100) для него
# чужое: оно слетает, awg0 остаётся — трафик доходит до сервера, но не попадает в
# sing-box (rx растёт, tx нет). Запрещаем networkd трогать чужие policy-rule.
if systemctl is-active --quiet systemd-networkd; then
  mkdir -p /etc/systemd/networkd.conf.d
  cat >/etc/systemd/networkd.conf.d/10-keep-foreign-rules.conf <<EOF
[Network]
ManageForeignRoutingPolicyRules=no
EOF
  systemctl restart systemd-networkd || true
fi
cat >/etc/nftables.conf <<EOF
#!/usr/sbin/nft -f
flush ruleset
table inet sbgw {
    chain input {
        type filter hook input priority filter; policy accept;
        iifname "lo" accept
        iifname "awg0" accept
        tcp dport $TPROXY_PORT drop
        udp dport $TPROXY_PORT drop
    }
    chain prerouting {
        type filter hook prerouting priority mangle; policy accept;
        iifname "awg0" ip daddr $TUN_CIDR return
        iifname "awg0" meta l4proto tcp tproxy ip to 127.0.0.1:$TPROXY_PORT meta mark set 1
        iifname "awg0" meta l4proto udp tproxy ip to 127.0.0.1:$TPROXY_PORT meta mark set 1
    }
}
EOF
systemctl enable --now nftables >/dev/null 2>&1
nft -f /etc/nftables.conf

say "интерфейс awg0 (пиры — из реестра peers/, мигрирует legacy при обновлении)"
bash "$WORK/peer-register.sh" rebuild

say "systemd: gateway + watchdog + таймер обновления"
cat >/etc/systemd/system/awg-gateway.service <<EOF
[Unit]
Description=AWG gateway policy routing for tproxy
After=network-online.target nftables.service awg-quick@awg0.service
Wants=network-online.target
Requires=awg-quick@awg0.service
[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=-/sbin/ip rule add fwmark 1 lookup 100
ExecStart=/sbin/ip route replace local 0.0.0.0/0 dev lo table 100
ExecStop=-/sbin/ip rule del fwmark 1 lookup 100
ExecStop=-/sbin/ip route del local 0.0.0.0/0 dev lo table 100
[Install]
WantedBy=multi-user.target
EOF
cat >/usr/local/bin/awg-watchdog.sh <<'EOF'
#!/bin/bash
# поднять awg0, если пропал
if ! /sbin/ip link show awg0 >/dev/null 2>&1; then
  logger -t awg-watchdog "awg0 missing, restarting"
  systemctl restart awg-quick@awg0 && systemctl restart awg-gateway
fi
# восстановить policy-routing tproxy: ip rule fwmark может быть сброшен
# переконфигурацией сети (networkd/cloud-init), а awg0 при этом остаётся —
# тогда трафик доходит до сервера, но не попадает в sing-box (rx растёт, tx нет)
if ! /sbin/ip rule show | grep -q "fwmark 0x1 lookup 100"; then
  logger -t awg-watchdog "fwmark rule missing, restoring"
  /sbin/ip rule add fwmark 1 lookup 100
fi
/sbin/ip route show table 100 2>/dev/null | grep -q "local default" || \
  /sbin/ip route replace local 0.0.0.0/0 dev lo table 100
EOF
chmod +x /usr/local/bin/awg-watchdog.sh
cat >/etc/systemd/system/awg-watchdog.service <<EOF
[Unit]
Description=AWG tunnel watchdog
[Service]
Type=oneshot
ExecStart=/usr/local/bin/awg-watchdog.sh
EOF
cat >/etc/systemd/system/awg-watchdog.timer <<EOF
[Unit]
Description=AWG watchdog every 60s
[Timer]
OnBootSec=90
OnUnitActiveSec=60
[Install]
WantedBy=timers.target
EOF
cat >/etc/systemd/system/sing-box-update.service <<EOF
[Unit]
Description=Update sing-box config from subscription
After=network-online.target
Wants=network-online.target
[Service]
Type=oneshot
ExecStart=/usr/bin/python3 /etc/sing-box/generate.py
ExecStartPost=/bin/systemctl reload-or-restart sing-box
EOF
cat >/etc/systemd/system/sing-box-update.timer <<EOF
[Unit]
Description=Periodic sing-box subscription update
[Timer]
OnBootSec=2min
OnUnitActiveSec=6h
Persistent=true
[Install]
WantedBy=timers.target
EOF

say "генерация конфига + запуск служб"
systemctl daemon-reload
python3 /etc/sing-box/generate.py
ip rule add fwmark 1 lookup 100 2>/dev/null || true
ip route replace local 0.0.0.0/0 dev lo table 100
systemctl enable awg-quick@awg0 awg-gateway sing-box >/dev/null 2>&1
awg-quick down awg0 2>/dev/null || true
systemctl restart awg-quick@awg0
systemctl restart awg-gateway
systemctl restart sing-box
systemctl enable --now awg-watchdog.timer sing-box-update.timer >/dev/null 2>&1

# ключи роутеров тут не печатаем — их выдаёт peer-register.sh register
echo "===VARS==="
echo "SERVER_PUB=$(cat server.pub)"
echo "AWG_PARAMS=$Jc $Jmin $Jmax $S1 $S2 $H1 $H2 $H3 $H4"
echo "PANEL_SECRET=$(cat /etc/sing-box/secret.txt)"
echo "===END==="
