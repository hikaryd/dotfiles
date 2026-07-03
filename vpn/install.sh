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

# SSH к Entware-шеллу роутера (root, порт 222) тем же паролем, что и веб-админка.
# Нужен один раз — записать netfilter.d-хук на флешку /opt (RCI этого не умеет).
router_ssh() {  # $1 = удалённая команда
  local cmd=$1
  local o="-p $ROUTER_SSH_PORT -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=10"
  if command -v sshpass >/dev/null 2>&1; then
    sshpass -p "$ROUTER_PASS" ssh $o "root@$ROUTER_HOST" "$cmd"
  elif command -v expect >/dev/null 2>&1; then
    ROUTER_PASS="$ROUTER_PASS" expect -c "
      set timeout 30
      spawn ssh $o root@$ROUTER_HOST {$cmd}
      expect {
        -re {[Pp]assword:} { send \"\$env(ROUTER_PASS)\r\"; exp_continue }
        -re {yes/no}       { send \"yes\r\"; exp_continue }
        eof
      }
      catch wait result
      exit [lindex \$result 3]
    "
  else
    return 127
  fi
}

# Ставит хук «весь LAN через туннель»: жёстко завернуть подсеть LAN в WG-таблицу.
# Это kill-switch для клиентов: упал WG → клиенты без инета (без утечки на WAN).
# Сам роутер при падении WG уходит на WAN по ping-check (для подписки/SSH).
install_lan_hook() {  # $1 = LAN-подсеть (CIDR), $2 = адрес роутера в туннеле (WG src)
  local lan="$1" wgsrc="$2" hook b64
  hook=$(cat <<HOOKEOF
#!/bin/sh
# LAN ($lan) -> VPN (Wireguard0 / nwg0). Идемпотентно; WG-таблицу находит
# динамически по источнику $wgsrc, переживает reboot (на каждый netfilter reload).
[ "\$type" = "ip6tables" ] && exit 0
LAN="$lan"
WGSRC="$wgsrc"
PRIO=1200
TBL=\$(ip rule list 2>/dev/null | sed -n "s/.*from \${WGSRC} lookup \\([0-9][0-9]*\\).*/\\1/p" | head -1)
[ -z "\$TBL" ] && exit 0
ip rule list 2>/dev/null | grep -q "from \${LAN} lookup \${TBL}" || ip rule add from \${LAN} lookup \${TBL} priority \${PRIO}
exit 0
HOOKEOF
)
  b64=$(printf '%s\n' "$hook" | base64 | tr -d '\n')
  router_ssh "echo $b64 | base64 -d > /opt/etc/ndm/netfilter.d/50-lan-via-wg && chmod +x /opt/etc/ndm/netfilter.d/50-lan-via-wg && type=iptables /opt/etc/ndm/netfilter.d/50-lan-via-wg && echo HOOK_OK"
}

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
  --router-name NAME   Имя роутера в реестре пиров VPS   (авто: hostname)
  --router-ssh-port N  SSH-порт Entware (root) для хука LAN→VPN       (222)
  --no-lan-hook        Не ставить хук «весь LAN через туннель»
  --awg-port PORT      UDP-порт AmneziaWG                          (51820)
  --tunnel-net CIDR    Подсеть туннеля (.1 сервер, .2 роутер) (10.8.2.0/24)
  --mtu N              MTU интерфейса                                (1420)
  --no-adblock         Не включать блокировку рекламы
  --dhcp-pool NAME     Имя DHCP-пула роутера                    (_WEBADMIN)
  --skip-vps           Пропустить настройку VPS (только роутер)
  --skip-router        Пропустить настройку роутера (только VPS)
  -h, --help           Эта справка

Примеры:
  ./install.sh --sub-url 'https://host/sub/TOKEN' --vps root@203.0.113.10 \\
               --router-pass 'СекретРоутера'

  # второй роутер на тот же сервер (запускать из сети второго роутера):
  ./install.sh --vps root@203.0.113.10 --router-pass 'Секрет' --skip-vps
EOF
}

# ─── параметры ───────────────────────────────────────────────────────
SUB_URL=""; VPS=""; ROUTER_HOST="192.168.254.1"; ROUTER_USER="admin"; ROUTER_PASS=""
ROUTER_NAME=""; ROUTER_CURPUB=""
ROUTER_SSH_PORT="222"; LAN_HOOK=1
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
    --router-name) ROUTER_NAME=$2; shift 2;;
    --router-ssh-port) ROUTER_SSH_PORT=$2; shift 2;;
    --no-lan-hook) LAN_HOOK=0; shift;;
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
[[ $SKIP_VPS -eq 1 && $SKIP_ROUTER -eq 1 ]] && die "нечего делать: --skip-vps и --skip-router одновременно"
if [[ $SKIP_VPS -eq 0 ]]; then
  [[ -z $SUB_URL ]] && SUB_URL=$(ask "Ссылка на подписку")
  [[ -z $SUB_URL ]] && die "Нужен --sub-url (или --skip-vps, если сервер уже развёрнут)"
fi
[[ -z $VPS ]] && VPS=$(ask "SSH к VPS (root@IP)")
[[ -z $VPS ]] && die "Нужен --vps"
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
for f in vps-setup.sh generate.py router-setup.py peer-register.sh; do
  [[ -f "$SCRIPT_DIR/$f" ]] || die "не найден $SCRIPT_DIR/$f"
done
ok "файлы установщика на месте"
# SSH к VPS нужен всегда: и для установки, и для регистрации пира роутера
ssh -o BatchMode=yes -o ConnectTimeout=10 "$VPS" 'echo ok' >/dev/null 2>&1 \
  || die "нет SSH-доступа к $VPS (нужен ключ/доступ root)"
ok "SSH к VPS ($SERVER_HOST) работает"
if [[ $SKIP_ROUTER -eq 0 ]]; then
  curl -s -m 6 -o /dev/null "http://$ROUTER_HOST/" || die "роутер $ROUTER_HOST недоступен по HTTP"
  ok "роутер $ROUTER_HOST отвечает"
  # имя роутера (для реестра пиров) + его текущий WG-ключ (для миграции/идемпотентности)
  IDENT=$(ROUTER_HOST="$ROUTER_HOST" ROUTER_USER="$ROUTER_USER" ROUTER_PASS="$ROUTER_PASS" \
          ACTION=identify python3 "$SCRIPT_DIR/router-setup.py") \
    || die "не удалось опросить роутер (проверь пароль веб-админки)"
  [[ -z $ROUTER_NAME ]] && ROUTER_NAME=$(sed -n 's/^NAME=//p' <<<"$IDENT" | head -1)
  ROUTER_CURPUB=$(sed -n 's/^CURPUB=//p' <<<"$IDENT" | head -1)
  [[ -n $ROUTER_NAME ]] || die "не удалось определить имя роутера — задай --router-name"
  ok "роутер опознан: $ROUTER_NAME"
fi

printf '\n%s   подписка:%s %s…\n' "$DIM" "$R" "${SUB_URL:0:42}"
printf '%s   VPS:%s %s   %sтуннель:%s %s (srv %s, cli %s)\n' "$DIM" "$R" "$SERVER_HOST" "$DIM" "$R" "$TUN_CIDR" "$TUN_SRV" "$TUN_CLI"
printf '%s   AWG-порт:%s %s   %sMTU:%s %s   %sadblock:%s %s\n' "$DIM" "$R" "$AWG_PORT" "$DIM" "$R" "$MTU" "$DIM" "$R" "$([[ $ADBLOCK == 1 ]] && echo вкл || echo выкл)"

# ─── VPS ─────────────────────────────────────────────────────────────
VARS_FILE=$(mktemp)
trap 'rm -f "$VARS_FILE"' EXIT  # в VARS бывает приватный ключ — не оставлять в /tmp
get_var() { sed -n "s/^$1=//p" "$VARS_FILE" | head -1; }
PANEL_SECRET=""
if [[ $SKIP_VPS -eq 0 ]]; then
  step "Настройка VPS ($SERVER_HOST)"
  scp -q "$SCRIPT_DIR/generate.py" "$VPS:/tmp/awg-generate.py"
  scp -q "$SCRIPT_DIR/vps-setup.sh" "$VPS:/tmp/awg-vps-setup.sh"
  scp -q "$SCRIPT_DIR/peer-register.sh" "$VPS:/tmp/awg-peer-register.sh"
  ok "файлы загружены на VPS, запускаю установку…"
  if ! ssh "$VPS" "env \
    SUB_URL='$SUB_URL' AWG_PORT='$AWG_PORT' TUN_CIDR='$TUN_CIDR' SRV_CIDR='$SRV_CIDR' \
    MTU='$MTU' ADBLOCK='$ADBLOCK' TPROXY_PORT='$TPROXY_PORT' \
    CLASH_PORT='$CLASH_PORT' ADBLOCK_WHITELIST='$ADBLOCK_WHITELIST' GEN_SRC=/tmp/awg-generate.py \
    PEERREG_SRC=/tmp/awg-peer-register.sh \
    bash /tmp/awg-vps-setup.sh" | tee "$VARS_FILE"; then
    die "установка на VPS завершилась с ошибкой (см. вывод выше)"
  fi
  grep -q '===VARS===' "$VARS_FILE" || die "установка VPS не дошла до конца (нет блока VARS)"
  PANEL_SECRET=$(get_var PANEL_SECRET)
  ok "VPS настроен"
fi

# ─── регистрация роутера в реестре пиров VPS ─────────────────────────
# У каждого роутера свой ключ и IP: один сервер обслуживает несколько роутеров,
# повторный запуск для того же роутера переиспользует его запись.
if [[ $SKIP_ROUTER -eq 0 ]]; then
  step "Регистрация роутера «$ROUTER_NAME» на VPS"
  ssh "$VPS" 'test -f /root/awgvpn/server.key' 2>/dev/null \
    || die "VPS не настроен (нет /root/awgvpn) — сначала запуск без --skip-vps"
  # свежая копия реестра — работает и с сервером, развёрнутым старой версией
  scp -q "$SCRIPT_DIR/peer-register.sh" "$VPS:/root/awgvpn/peer-register.sh"
  ssh "$VPS" "bash /root/awgvpn/peer-register.sh register '$ROUTER_NAME' '$ROUTER_CURPUB'" > "$VARS_FILE" \
    || die "регистрация пира на VPS не удалась"
  grep -q '===VARS===' "$VARS_FILE" || die "реестр пиров не вернул параметры (нет блока VARS)"
  SERVER_PUB=$(get_var SERVER_PUB)
  ROUTER_KEY=$(get_var ROUTER_KEY)
  AWG_PARAMS=$(get_var AWG_PARAMS)
  PANEL_SECRET=$(get_var PANEL_SECRET)
  TUN_CLI=$(get_var ROUTER_IP)
  # параметры сервера авторитетны (важно при --skip-vps с нестандартным портом/подсетью)
  AWG_PORT=$(get_var AWG_PORT); AWG_PORT=${AWG_PORT:-51820}
  MTU=$(get_var MTU); MTU=${MTU:-1280}
  SRV_TUN=$(get_var TUN_SRV); [[ -n $SRV_TUN ]] && TUN_SRV=$SRV_TUN
  SRV_NET=$(get_var TUN_CIDR); [[ -n $SRV_NET ]] && TUN_CIDR=$SRV_NET
  [[ -n $SERVER_PUB && -n $ROUTER_KEY && -n $AWG_PARAMS && -n $TUN_CLI ]] \
    || die "не удалось получить ключи/параметры AWG из реестра"
  ok "пир зарегистрирован: $ROUTER_NAME → $TUN_CLI"
fi
rm -f "$VARS_FILE"

# ─── роутер ──────────────────────────────────────────────────────────
if [[ $SKIP_ROUTER -eq 0 ]]; then
  step "Настройка роутера Keenetic ($ROUTER_HOST)"
  # shellcheck disable=SC2097,SC2098  # одноимённый проброс env в python — значения из родителя
  ROUTER_HOST="$ROUTER_HOST" ROUTER_USER="$ROUTER_USER" ROUTER_PASS="$ROUTER_PASS" \
  ROUTER_LAN_IP="$ROUTER_HOST" SERVER_HOST="$SERVER_HOST" AWG_PORT="$AWG_PORT" \
  TUN_CLI="$TUN_CLI" TUN_SRV="$TUN_SRV" TUN_MASK="$TUN_MASK" MTU="$MTU" \
  SERVER_PUB="$SERVER_PUB" ROUTER_KEY="$ROUTER_KEY" AWG_PARAMS="$AWG_PARAMS" \
  DHCP_POOL="$DHCP_POOL" \
    python3 "$SCRIPT_DIR/router-setup.py" || die "настройка роутера не удалась"
  ok "роутер настроен"

  # весь домашний сегмент через туннель (RCI/ip global этого для форварда не делает)
  if [[ $LAN_HOOK -eq 1 ]]; then
    LAN_SUBNET="${ROUTER_HOST%.*}.0/24"
    if install_lan_hook "$LAN_SUBNET" "$TUN_CLI" 2>/dev/null | grep -q HOOK_OK; then
      ok "хук LAN→VPN установлен ($LAN_SUBNET → туннель, переживёт reboot)"
    else
      warn "не удалось поставить хук LAN→VPN по SSH (root@$ROUTER_HOST:$ROUTER_SSH_PORT)"
      warn "без него клиенты идут мимо VPN; проверь SSH к Entware или поставь вручную"
    fi
  fi
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
   ${B}Роутер:${R}           $([[ $SKIP_ROUTER == 0 ]] && echo "$ROUTER_NAME → $TUN_CLI (реестр пиров: peer-register.sh list на VPS)" || echo "пропущен (--skip-router)")
   ${B}Маршрутизация:${R}    РФ напрямую · остальное → VPN · adblock $([[ $ADBLOCK == 1 ]] && echo вкл || echo выкл)
   ${B}Клиенты LAN:${R}      $([[ $LAN_HOOK == 1 ]] && echo "весь сегмент → туннель, kill-switch (хук netfilter.d)" || echo "хук отключён (--no-lan-hook); маршрут по ip global")
   ${B}Failover:${R}         $([[ $LAN_HOOK == 1 ]] && echo "роутер → WAN по ping-check; клиенты — kill-switch (без утечки)" || echo "при падении туннеля — автопереключение на WAN")

   ${DIM}Подписка обновляется сама (таймер 6ч). На устройствах обнови
   аренду DHCP (переподключи Wi-Fi), чтобы получить чистый DNS.${R}
EOF
