#!/usr/bin/env bash
# shellcheck disable=SC2154  # Jc/Jmin/Jmax/S1/S2/H1..H4 приходят из awg_params.env
# Реестр AWG-пиров (роутеров) на VPS. Выполняется НА VPS, вызывается install.sh
# и uninstall.sh по SSH. Позволяет вешать несколько роутеров на один сервер:
# у каждого — свой ключ и свой IP в туннельной подсети (peers/<имя>/{key,pub,ip}).
#
#   peer-register.sh register ИМЯ [ТЕКУЩИЙ_PUBKEY]   зарегистрировать/переиспользовать пир
#   peer-register.sh remove ИМЯ                       удалить пир
#   peer-register.sh list                             список пиров
#   peer-register.sh rebuild                          только пересобрать awg0.conf
#
# register печатает блок ===VARS=== с данными для настройки роутера.
# Живое применение через awg syncconf — без обрыва остальных роутеров.
set -euo pipefail

WORK=${AWG_WORK:-/root/awgvpn}
PEERS=$WORK/peers
CONF=${AWG_CONF:-/etc/amnezia/amneziawg/awg0.conf}

# say → stderr: stdout register'а парсится install.sh (блок ===VARS===)
say() { printf '   \033[36m·\033[0m %s\n' "$*" >&2; }
die() { printf 'ОШИБКА: %s\n' "$*" >&2; exit 1; }

[ -d "$WORK" ] && [ -f "$WORK/server.key" ] || die "VPS не настроен ($WORK/server.key нет) — сначала полный install.sh"
cd "$WORK"; umask 077
# защита от параллельных запусков (гонка за IP / запись awg0.conf);
# flock есть на Debian/Ubuntu (util-linux) — на macOS (локальные тесты) его нет
exec 9>"$WORK/.lock"
command -v flock >/dev/null 2>&1 && flock 9
# shellcheck disable=SC1091,SC2154  # Jc/Jmin/… приходят из awg_params.env
. ./awg_params.env

# server.env пишет vps-setup.sh; на серверах, развёрнутых старой версией,
# восстанавливаем параметры из существующего awg0.conf и сохраняем.
ensure_server_env() {
  if [ ! -f server.env ]; then
    [ -f "$CONF" ] || die "нет ни server.env, ни $CONF — не из чего взять параметры туннеля"
    local addr port mtu
    addr=$(sed -n 's/^Address *= *//p' "$CONF" | head -1)
    port=$(sed -n 's/^ListenPort *= *//p' "$CONF" | head -1)
    mtu=$(sed -n 's/^MTU *= *//p' "$CONF" | head -1)
    [ -n "$addr" ] && [ -n "$port" ] || die "не удалось разобрать $CONF"
    cat >server.env <<EOF
SRV_CIDR=$addr
AWG_PORT=$port
MTU=${mtu:-1280}
TUN_CIDR=${addr%.*}.0/${addr#*/}
EOF
  fi
  # shellcheck disable=SC1091
  . ./server.env
}

# Сервер, развёрнутый старой версией, держит единственный ключ роутера в
# router.key — переносим его в реестр как _legacy, чтобы первый роутер
# продолжил работать со своим ключом/IP.
migrate_legacy() {
  [ -f router.key ] && [ ! -d "$PEERS" ] || return 0
  say "миграция старого пира в реестр (peers/_legacy)"
  # router.pub мог потеряться (оборванная установка) — восстановим из ключа,
  # иначе mv упадёт на середине и rebuild молча выкинет пир первого роутера
  [ -f router.pub ] || wg pubkey <router.key >router.pub
  mkdir -p "$PEERS/_legacy"
  mv router.key "$PEERS/_legacy/key"
  mv router.pub "$PEERS/_legacy/pub"
  local ip="" base=${TUN_CIDR%/*}
  base=${base%.*}
  [ -f "$CONF" ] && ip=$(sed -n 's|^AllowedIPs *= *\([0-9.]*\)/32.*|\1|p' "$CONF" | head -1)
  echo "${ip:-$base.2}" >"$PEERS/_legacy/ip"
}

# Имя пира = ASCII-слаг; если после чистки пусто (кириллица и т.п.) —
# детерминированный fallback от исходной строки (идемпотентность сохраняется).
sanitize() {
  local raw=$1 s
  s=$(printf '%s' "$raw" | tr -c 'A-Za-z0-9_-' '-' | sed 's/^-*//; s/-*$//' | cut -c1-48)
  [ -n "$s" ] || s="router-$(printf '%s' "$raw" | cksum | cut -d' ' -f1)"
  printf '%s' "$s"
}

next_free_ip() {
  local base=${TUN_CIDR%/*}; base=${base%.*}
  local i ip
  for i in $(seq 2 254); do
    ip="$base.$i"
    grep -qxF "$ip" "$PEERS"/*/ip 2>/dev/null || { echo "$ip"; return 0; }
  done
  die "в подсети $TUN_CIDR не осталось свободных адресов"
}

rebuild_conf() {
  mkdir -p "$(dirname "$CONF")"
  # пишем во временный файл + mv: смерть посреди записи не оставит битый конфиг
  local tmp="$CONF.tmp.$$"
  {
    cat <<EOF
[Interface]
Address = $SRV_CIDR
ListenPort = $AWG_PORT
PrivateKey = $(cat server.key)
Jc = $Jc
Jmin = $Jmin
Jmax = $Jmax
S1 = $S1
S2 = $S2
H1 = $H1
H2 = $H2
H3 = $H3
H4 = $H4
MTU = $MTU
EOF
    local d
    for d in "$PEERS"/*/; do
      [ -f "$d/pub" ] && [ -f "$d/ip" ] || continue
      printf '\n# peer: %s\n[Peer]\nPublicKey = %s\nAllowedIPs = %s/32\n' \
        "$(basename "$d")" "$(cat "$d/pub")" "$(cat "$d/ip")"
    done
  } >"$tmp"
  mv "$tmp" "$CONF"
}

# Применить конфиг к живому интерфейсу, не роняя чужие сессии.
apply_live() {
  ip link show awg0 >/dev/null 2>&1 || return 0
  command -v awg >/dev/null 2>&1 || return 0
  if ! awg syncconf awg0 <(awg-quick strip awg0) 2>/dev/null; then
    systemctl restart awg-quick@awg0 2>/dev/null || true
  fi
}

cmd_register() {
  local name curpub d
  name=$(sanitize "${1:?нужно имя роутера}")
  curpub=${2:-}
  [ -n "$name" ] || die "пустое имя роутера после нормализации"
  mkdir -p "$PEERS"

  # Adoption: роутер уже настроен со старым ключом (например, из peers/_legacy),
  # но регистрируется под новым именем — переносим его записи, а не плодим пир.
  if [ ! -d "$PEERS/$name" ] && [ -n "$curpub" ]; then
    for d in "$PEERS"/*/; do
      [ -f "$d/pub" ] || continue
      if [ "$(cat "$d/pub")" = "$curpub" ]; then
        say "пир с этим ключом уже есть ($(basename "$d")) — переименовываю в '$name'"
        mv "${d%/}" "$PEERS/$name"
        break
      fi
    done
  fi

  if [ ! -d "$PEERS/$name" ]; then
    say "новый пир '$name'"
    mkdir -p "$PEERS/$name"
    wg genkey >"$PEERS/$name/key"
    wg pubkey <"$PEERS/$name/key" >"$PEERS/$name/pub"
    next_free_ip >"$PEERS/$name/ip"
  else
    say "пир '$name' уже зарегистрирован — использую его ключ и IP"
  fi

  rebuild_conf
  apply_live
  say "пир '$name' → $(cat "$PEERS/$name/ip")"

  echo "===VARS==="
  echo "SERVER_PUB=$(cat server.pub)"
  echo "ROUTER_KEY=$(cat "$PEERS/$name/key")"
  echo "ROUTER_IP=$(cat "$PEERS/$name/ip")"
  echo "AWG_PARAMS=$Jc $Jmin $Jmax $S1 $S2 $H1 $H2 $H3 $H4"
  # параметры сервера — авторитетны для роутера (важно при --skip-vps)
  echo "AWG_PORT=$AWG_PORT"
  echo "MTU=$MTU"
  echo "TUN_SRV=${SRV_CIDR%/*}"
  echo "TUN_CIDR=$TUN_CIDR"
  echo "PANEL_SECRET=$(cat /etc/sing-box/secret.txt 2>/dev/null || true)"
  echo "===END==="
}

cmd_remove() {
  local name
  name=$(sanitize "${1:?нужно имя роутера}")
  if [ ! -d "$PEERS/$name" ]; then
    say "пир '$name' не найден — нечего удалять"
    return 0
  fi
  rm -rf "${PEERS:?}/$name"
  rebuild_conf
  apply_live
  say "пир '$name' удалён"
}

cmd_list() {
  local d
  [ -d "$PEERS" ] || { say "пиров нет"; return 0; }
  for d in "$PEERS"/*/; do
    [ -f "$d/ip" ] || continue
    printf '%s\t%s\t%s\n' "$(basename "$d")" "$(cat "$d/ip")" "$(cat "$d/pub" 2>/dev/null)"
  done
}

ensure_server_env
migrate_legacy

case "${1:-}" in
  register) shift; cmd_register "$@";;
  remove)   shift; cmd_remove "$@";;
  list)     cmd_list;;
  rebuild)  rebuild_conf; apply_live; say "awg0.conf пересобран";;
  *) die "использование: $0 register ИМЯ [PUBKEY] | remove ИМЯ | list | rebuild";;
esac
