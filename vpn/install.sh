#!/usr/bin/env bash
# ╔══════════════════════════════════════════════════════════════════╗
# ║  AmneziaWG + sing-box split-VPN — установщик «под ключ»           ║
# ║  Дом → Keenetic(AWG) → VPS → sing-box → подписка → интернет       ║
# ║  РФ напрямую · остальное через VPN · adblock · панель · failover  ║
# ╚══════════════════════════════════════════════════════════════════╝
set -euo pipefail

# ─── оформление ──────────────────────────────────────────────────────
if [[ -t 1 ]]; then
  B=$'\033[1m'; DIM=$'\033[2m'; R=$'\033[0m'
  RED=$'\033[31m'; GRN=$'\033[32m'; YEL=$'\033[33m'; BLU=$'\033[34m'; CYA=$'\033[36m'
else
  B=''; DIM=''; R=''; RED=''; GRN=''; YEL=''; BLU=''; CYA=''
fi
step() { printf '\n%s━━ %s %s\n' "$BLU$B" "$*" "$R"; }
ok()   { printf '   %s✓%s %s\n' "$GRN" "$R" "$*"; }
warn() { printf '   %s!%s %s\n' "$YEL" "$R" "$*"; }
die()  { printf '\n%s✗ %s%s\n' "$RED$B" "$*" "$R" >&2; exit 1; }
ask()  { local p=$1 d=${2:-} a; if [[ -n $d ]]; then read -rp "   $p [$d]: " a; echo "${a:-$d}"; else read -rp "   $p: " a; echo "$a"; fi; }

banner() {
  printf '%s' "$CYA$B"
  cat <<'ART'
   ┌────────────────────────────────────────────────┐
   │   AmneziaWG · sing-box · split-VPN  installer    │
   └────────────────────────────────────────────────┘
ART
  printf '%s' "$R"
}

usage() {
  cat <<EOF
${B}Использование:${R} ./install.sh [опции]

  --sub-url URL        Ссылка на подписку (vless://… список)        [обяз.]
  --vps TARGET         SSH к VPS, напр. root@203.0.113.10           [обяз.]
  --router-host IP     Адрес роутера Keenetic            (192.168.254.1)
  --router-user USER   Логин веб-админки роутера                 (admin)
  --router-pass PASS   Пароль веб-админки роутера                 [обяз. для роутера]
  --awg-port PORT      UDP-порт AmneziaWG                          (51820)
  --tunnel-net CIDR    Подсеть туннеля (.1 сервер, .2 роутер) (10.8.2.0/24)
  --mtu N              MTU интерфейса                                (1420)
  --no-adblock         Не включать блокировку рекламы
  --dhcp-pool NAME     Имя DHCP-пула роутера                    (_WEBADMIN)
  --skip-vps           Пропустить настройку VPS (только роутер)
  --skip-router        Пропустить настройку роутера (только VPS)
  -h, --help           Эта справка

Пример:
  ./install.sh --sub-url 'https://host/sub/TOKEN' --vps root@203.0.113.10 \\
               --router-pass 'СекретРоутера'
EOF
}

# ─── параметры ───────────────────────────────────────────────────────
SUB_URL=""; VPS=""; ROUTER_HOST="192.168.254.1"; ROUTER_USER="admin"; ROUTER_PASS=""
AWG_PORT="51820"; TUN_NET="10.8.2.0/24"; MTU="1280"; ADBLOCK="1"; DHCP_POOL="_WEBADMIN"
SKIP_VPS=0; SKIP_ROUTER=0
ADBLOCK_WHITELIST="tmdb.org,themoviedb.org,b-cdn.net"; TPROXY_PORT="7895"; CLASH_PORT="9090"

while [[ $# -gt 0 ]]; do
  case $1 in
    --sub-url) SUB_URL=$2; shift 2;;
    --vps) VPS=$2; shift 2;;
    --router-host) ROUTER_HOST=$2; shift 2;;
    --router-user) ROUTER_USER=$2; shift 2;;
    --router-pass) ROUTER_PASS=$2; shift 2;;
    --awg-port) AWG_PORT=$2; shift 2;;
    --tunnel-net) TUN_NET=$2; shift 2;;
    --mtu) MTU=$2; shift 2;;
    --no-adblock) ADBLOCK="0"; shift;;
    --dhcp-pool) DHCP_POOL=$2; shift 2;;
    --skip-vps) SKIP_VPS=1; shift;;
    --skip-router) SKIP_ROUTER=1; shift;;
    -h|--help) usage; exit 0;;
    *) die "Неизвестная опция: $1";;
  esac
done

banner
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

# ─── интерактивный доспрос ───────────────────────────────────────────
[[ -z $SUB_URL ]] && SUB_URL=$(ask "Ссылка на подписку")
[[ -z $VPS ]]     && VPS=$(ask "SSH к VPS (root@IP)")
[[ -z $SUB_URL || -z $VPS ]] && die "Нужны --sub-url и --vps"
if [[ $SKIP_ROUTER -eq 0 && -z $ROUTER_PASS ]]; then
  ROUTER_PASS=$(ask "Пароль роутера ($ROUTER_USER@$ROUTER_HOST)")
fi

# производные адреса туннеля (предполагается /24): 10.8.2.0/24 -> 10.8.2
TUN_BASE=${TUN_NET%/*}; TUN_BASE=${TUN_BASE%.*}
TUN_SRV="${TUN_BASE}.1"; TUN_CLI="${TUN_BASE}.2"
TUN_CIDR="${TUN_BASE}.0/24"; SRV_CIDR="${TUN_SRV}/24"; TUN_MASK="255.255.255.0"
SERVER_HOST=${VPS##*@}   # IP без user@

# ─── preflight ───────────────────────────────────────────────────────
step "Проверки окружения"
for b in ssh scp python3 curl; do command -v "$b" >/dev/null || die "нет утилиты: $b"; done
ok "локальные утилиты на месте"
for f in vps-setup.sh generate.py router-setup.py; do
  [[ -f "$SCRIPT_DIR/$f" ]] || die "не найден $SCRIPT_DIR/$f"
done
ok "файлы установщика на месте"
if [[ $SKIP_VPS -eq 0 ]]; then
  ssh -o BatchMode=yes -o ConnectTimeout=10 "$VPS" 'echo ok' >/dev/null 2>&1 \
    || die "нет SSH-доступа к $VPS (нужен ключ/доступ root)"
  ok "SSH к VPS ($SERVER_HOST) работает"
fi
if [[ $SKIP_ROUTER -eq 0 ]]; then
  curl -s -m 6 -o /dev/null "http://$ROUTER_HOST/" || die "роутер $ROUTER_HOST недоступен по HTTP"
  ok "роутер $ROUTER_HOST отвечает"
fi

printf '\n%s   подписка:%s %s…\n' "$DIM" "$R" "${SUB_URL:0:42}"
printf '%s   VPS:%s %s   %sтуннель:%s %s (srv %s, cli %s)\n' "$DIM" "$R" "$SERVER_HOST" "$DIM" "$R" "$TUN_CIDR" "$TUN_SRV" "$TUN_CLI"
printf '%s   AWG-порт:%s %s   %sMTU:%s %s   %sadblock:%s %s\n' "$DIM" "$R" "$AWG_PORT" "$DIM" "$R" "$MTU" "$DIM" "$R" "$([[ $ADBLOCK == 1 ]] && echo вкл || echo выкл)"

# ─── VPS ─────────────────────────────────────────────────────────────
VARS_FILE=$(mktemp)
if [[ $SKIP_VPS -eq 0 ]]; then
  step "Настройка VPS ($SERVER_HOST)"
  scp -q "$SCRIPT_DIR/generate.py" "$VPS:/tmp/awg-generate.py"
  scp -q "$SCRIPT_DIR/vps-setup.sh" "$VPS:/tmp/awg-vps-setup.sh"
  ok "файлы загружены на VPS, запускаю установку…"
  ssh "$VPS" "env \
    SUB_URL='$SUB_URL' AWG_PORT='$AWG_PORT' TUN_CIDR='$TUN_CIDR' SRV_CIDR='$SRV_CIDR' \
    TUN_CLI='$TUN_CLI' MTU='$MTU' ADBLOCK='$ADBLOCK' TPROXY_PORT='$TPROXY_PORT' \
    CLASH_PORT='$CLASH_PORT' ADBLOCK_WHITELIST='$ADBLOCK_WHITELIST' GEN_SRC=/tmp/awg-generate.py \
    bash /tmp/awg-vps-setup.sh" | tee "$VARS_FILE"
  grep -q '===VARS===' "$VARS_FILE" || die "установка VPS не дошла до конца (нет блока VARS)"
  ok "VPS настроен"
else
  step "Пропуск VPS — забираю параметры с $SERVER_HOST"
  ssh "$VPS" 'cd /root/awgvpn && . ./awg_params.env && {
    echo "===VARS==="
    echo "SERVER_PUB=$(cat server.pub)"
    echo "ROUTER_KEY=$(cat router.key)"
    echo "AWG_PARAMS=$Jc $Jmin $Jmax $S1 $S2 $H1 $H2 $H3 $H4"
    echo "PANEL_SECRET=$(cat /etc/sing-box/secret.txt)"
    echo "===END==="
  }' > "$VARS_FILE" || die "не удалось забрать параметры с VPS"
  ok "параметры получены"
fi

# распарсить VARS
get_var() { sed -n "s/^$1=//p" "$VARS_FILE" | head -1; }
SERVER_PUB=$(get_var SERVER_PUB)
ROUTER_KEY=$(get_var ROUTER_KEY)
AWG_PARAMS=$(get_var AWG_PARAMS)
PANEL_SECRET=$(get_var PANEL_SECRET)
rm -f "$VARS_FILE"
[[ -n $SERVER_PUB && -n $ROUTER_KEY && -n $AWG_PARAMS ]] || die "не удалось получить ключи/параметры AWG"

# ─── роутер ──────────────────────────────────────────────────────────
if [[ $SKIP_ROUTER -eq 0 ]]; then
  step "Настройка роутера Keenetic ($ROUTER_HOST)"
  ROUTER_HOST="$ROUTER_HOST" ROUTER_USER="$ROUTER_USER" ROUTER_PASS="$ROUTER_PASS" \
  ROUTER_LAN_IP="$ROUTER_HOST" SERVER_HOST="$SERVER_HOST" AWG_PORT="$AWG_PORT" \
  TUN_CLI="$TUN_CLI" TUN_SRV="$TUN_SRV" TUN_MASK="$TUN_MASK" MTU="$MTU" \
  SERVER_PUB="$SERVER_PUB" ROUTER_KEY="$ROUTER_KEY" AWG_PARAMS="$AWG_PARAMS" \
  DHCP_POOL="$DHCP_POOL" \
    python3 "$SCRIPT_DIR/router-setup.py" || die "настройка роутера не удалась"
  ok "роутер настроен"
fi

# ─── проверка ────────────────────────────────────────────────────────
step "Проверка"
if curl -s -m 8 -o /dev/null -w '%{http_code}' "http://$SERVER_HOST:$CLASH_PORT/ui/" | grep -q 200; then
  ok "панель доступна"
else
  warn "панель пока не отвечает (проверь позже)"
fi
EXIT_IP=$(curl -s -m 10 https://api.ipify.org 2>/dev/null || true)
[[ -n $EXIT_IP ]] && printf '   %sтекущий выходной IP:%s %s\n' "$DIM" "$R" "$EXIT_IP"

# ─── итог ────────────────────────────────────────────────────────────
step "Готово"
cat <<EOF
   ${GRN}${B}Стек развёрнут.${R}

   ${B}Панель sing-box:${R}  http://$SERVER_HOST:$CLASH_PORT/ui/
   ${B}Пароль панели:${R}    $PANEL_SECRET
   ${B}Туннель:${R}          $SERVER_HOST:$AWG_PORT  (AmneziaWG, $TUN_CIDR)
   ${B}Маршрутизация:${R}    РФ напрямую · остальное → VPN · adblock $([[ $ADBLOCK == 1 ]] && echo вкл || echo выкл)
   ${B}Failover:${R}         при падении туннеля — автопереключение на WAN

   ${DIM}Подписка обновляется сама (таймер 6ч). На устройствах обнови
   аренду DHCP (переподключи Wi-Fi), чтобы получить чистый DNS.${R}
EOF
