# AmneziaWG + sing-box split-VPN

Развёртывание домашнего VPN-шлюза «под ключ»:

```
Дом → Keenetic (AmneziaWG) → VPS → sing-box → подписка (VLESS/Reality) → интернет
        │                      │
        │ РФ-трафик            └ geoip-ru → direct (выход с IP VPS)
        │ напрямую через WAN     остальное → подписка (заграничный выход)
        └ при падении туннеля → failover на WAN
```

Возможности:
- **AmneziaWG** между роутером и VPS (обфускация WG против DPI).
- **sing-box** на VPS как прозрачный tproxy-шлюз: подписка `vless://…` → outbound'ы с авто-выбором быстрейшего узла.
- **Split-routing**: РФ (`geoip-ru`) идёт напрямую с IP VPS, всё остальное — через подписку.
- **Adblock** на уровне DNS (geosite ad-list) с whitelist для ложных срабатываний (TMDB/BunnyCDN).
- **Чистый DNS** через туннель — обход DNS-отравления РФ-провайдеров.
- **Панель** (zashboard / Clash API) — переключение узлов, пинги, трафик, логи.
- **Failover** на WAN через ping-check + самовосстановление (watchdog, авто-рестарт).
- **Авто-обновление** подписки (systemd-таймер, 6 ч).

## Требования

- **VPS**: Debian/Ubuntu, root по SSH (по ключу), внешний UDP-порт доступен (1:1 NAT ок).
- **Роутер**: Keenetic/Netcraze, KeeneticOS **5.x** (нужна поддержка `wireguard asc`), доступ к веб-админке.
- **Локально**: `bash`, `ssh`, `scp`, `python3`, `curl`. Запускать с устройства в сети роутера.

## Установка

```bash
./install.sh \
  --sub-url 'https://noctuvpn.xyz/get_subscriptions/ТОКЕН' \
  --vps root@203.0.113.10 \
  --router-pass 'ПарольРоутера'
```

Без аргументов скрипт спросит обязательные параметры интерактивно. Все опции — `./install.sh --help`.

Частые сценарии:

```bash
# только сервер (роутер настрою позже)
./install.sh --sub-url URL --vps root@IP --skip-router

# только роутер (сервер уже развёрнут — ключи возьмутся с VPS)
./install.sh --vps root@IP --router-pass PASS --skip-vps

# свой порт/подсеть/без adblock
./install.sh --sub-url URL --vps root@IP --router-pass PASS \
  --awg-port 51820 --tunnel-net 10.8.2.0/24 --no-adblock
```

## Файлы

| Файл | Назначение |
|------|-----------|
| `install.sh` | Оркестратор: аргументы, preflight, запуск частей, проверка, итог |
| `vps-setup.sh` | Серверная часть (выполняется на VPS): AWG + sing-box + nftables + службы |
| `generate.py` | Генератор `config.json` sing-box из подписки (читает `/etc/sing-box/vpn.env`) |
| `router-setup.py` | Настройка Keenetic через HTTP RCI (AWG-клиент, ip global, ping-check, DNS) |

## После установки

- Панель: `http://<VPS_IP>:9090/ui/`, пароль печатается в конце установки (и лежит в `/etc/sing-box/secret.txt` на VPS). Открывать по **http** (не https).
- На устройствах обнови аренду DHCP (переподключи Wi-Fi), чтобы получить роутер как DNS — иначе кэш старого DNS может мешать.

### Управление (на VPS)

```bash
# сменить подписку / вкл-выкл adblock
nano /etc/sing-box/vpn.env        # SUB_URL=… , ADBLOCK=1|0
python3 /etc/sing-box/generate.py && systemctl restart sing-box

# статус
systemctl status sing-box awg-quick@awg0 awg-gateway
awg show awg0
```

### Whitelist adblock

Если adblock ложно блокирует домен (картинки/CDN не грузятся) — добавь его в
`ADBLOCK_WHITELIST` в `/etc/sing-box/vpn.env` (через запятую) и перегенерируй конфиг.

## Заметки

- Во время failover (туннель упал → WAN) трафик идёт напрямую — компромисс ради связи.
- РФ-сайты на зарубежных CDN (Cloudflare и т.п.) пойдут через VPN — ограничение geoip.
- `xhttp`-узлы из подписки пропускаются (sing-box их не поддерживает).
