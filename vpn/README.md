# VPN-шлюз

AmneziaWG между роутером и Debian/Ubuntu VPS; sing-box разделяет маршруты.
Нужны SSH по ключу и Keenetic/Netcraze с HTTP RCI, доступный напрямую.
Локально: Bash, SSH/SCP, curl и Go 1.26+. Собственные helpers — `dots-vpn`.

```sh
./scripts/build-go-tools.sh
./vpn/install.sh --help
./vpn/install.sh
```

Установщик запрашивает адреса и секреты. Меняет VPS/роутер: проверьте цели и
сохраните резервные копии. `--skip-router` пропускает роутер; `--skip-vps`
использует существующий сервер и сохраняет пиров. Не повторяйте полную установку
вслепую после обрыва SSH. Для второго дома нужен `--skip-vps` и отдельный пир.
Секреты — вне Git; [правила хранения](../docs/PRIVATE.md).

На VPS native binary установлен в `/etc/sing-box/dots-vpn`, настройки —
`/etc/sing-box/vpn.env`, секрет панели — `/etc/sing-box/secret.txt`, пиры —
`/root/awgvpn/peers/`. Обновление кода в репозитории само не обновляет VPS.

## Маршруты и наблюдение

- `AWG_LAN` назначает выбранному сегменту туннель; другие сегменты не меняются.
  Индивидуальные правила приоритетнее. WAN fallback, IPv6 и утечки проверяются
  на конкретном роутере; старые сторонние хуки автоматически не удаляются.
- Ручной selector сохраняет все поддерживаемые узлы и имена. Auto по умолчанию
  содержит до 8 уникальных конфигураций без LTE/3G/4G и «Обход БС».
  `AUTO_INCLUDE`/`AUTO_EXCLUDE` — regex без учёта регистра, `AUTO_MAX` — лимит.
  `AUTO_PREFER` передвигает совпавшие имена перед дедупликацией/лимитом,
  сохраняя порядок внутри групп. Пустой `AUTO_EXCLUDE=` отключает фильтр.
  Эти параметры сохраняются при переустановке; пустой/невалидный пул не заменяет
  рабочий конфиг. Лимит проверок не обещает максимальной скорости.
- Поддерживаются VLESS и Hysteria2/hy2 с TLS/SNI/obfs из URI. XHTTP sing-box
  здесь не поддерживает: причина и индекс пропуска выводятся явно.
- `systemctl status sing-box awg-quick@awg0 awg-gateway` — службы.
  `vpn-health.timer` раз в 2 минуты проверяет HTTPS через proxy, без direct
  fallback и перезапусков. Это не проверка Wi-Fi, proxy-dns или всей цепочки LAN.
  Диагностика: `journalctl -u vpn-health.service`; localhost `HEALTH_PORT=7896`.
- Панель: `http://10.8.2.1:9090/ui/` через туннель, пароль обязателен.
  Снаружи: `ssh -N -L 9090:127.0.0.1:9090 root@VPS` и localhost URL.
  Не открывайте HTTP-панель в недоверенную сеть. MTU остаётся 1280.

## Измерения

Выполняются **на VPS**. Helpers запускают отдельный авторизованный localhost
proxy, не меняют рабочие selector/службы/firewall и очищают свои процессы/файлы.
Нужны sing-box и curl. Свежие узлы экспортируются без изменения gateway:

```sh
umask 077
VPN=/etc/sing-box/dots-vpn
VPN_BENCH_CONFIG=$(mktemp)
trap 'rm -f "$VPN_BENCH_CONFIG"' EXIT
"$VPN" nodes --output "$VPN_BENCH_CONFIG"
"$VPN" benchmark --config "$VPN_BENCH_CONFIG" --all
"$VPN" benchmark --config "$VPN_BENCH_CONFIG" --only 'точное имя узла' \
  --samples 3 --bytes 10485760 --timeout 20 \
  --url 'https://speed.cloudflare.com/__down?bytes={bytes}' \
  --url 'https://proof.ovh.net/files/10Mb.dat'
```

`--all` проверяет все proxy outbounds переданного файла, не скачивает подписку.
Сопоставьте число разобранных строк и причины пропусков. Defaults: `--all` —
1 × 1 MB / 10 s; обычный auto + direct — 2 × 10 MB / 20 s. `--only` повторяется
для финалистов; `--extra` добавляет ручной узел. До 4 HTTPS URL, нужен HTTP 200
и точный размер; `{bytes}` подставляется. Секреты в URL не используйте.
Лимиты: samples 1–5, bytes 1–50000000, timeout 1–30 s; общий `--max-duration`
до 86400 s. Трафик примерно `(узлы + direct) × samples × bytes × URL`.
Exit 1 — отсутствующие успешные пары/неполный прогон. Это VPS→узел→источник,
не скорость LAN/Wi-Fi/AWG; финалистов проверяют отдельно из дома.

XHTTP проверяется отдельно, без подмены на TCP и включения в рабочий auto:

```sh
"$VPN" xhttp-benchmark --source-index "$XHTTP_INDEX" --check-only
"$VPN" xhttp-benchmark --source-index "$XHTTP_INDEX" \
  --samples 3 --bytes 10485760 --timeout 20 --max-duration 240
```

Индекс — с нуля по свежей сырой подписке, включая пустые строки. При перестановке
строк проверьте его заново. На Linux amd64 скачивается Xray 26.3.27 с SHA256
проверкой в native cache; `--install-only` лишь готовит его. Любая failed/skipped
проба даёт exit 1. Один удачный пик не доказывает общего ускорения.

## TCP tuning

После замера: `/etc/sing-box/dots-vpn tcp-tuning install --peer 10.8.2.2/32`.
IP должен совпадать с AllowedIP пира `awg0`; повторяемые `--peer` задают полный
список. Без флага сохраняется список из `/etc/awg-tcp-tuning.json`; новый — пуст.
Служба восстанавливает свои маршруты после AWG restart, установка AWG не
перезапускает. Глобальные Cubic/qdisc и остальные пиры не меняются. Откат:
`/usr/local/libexec/dots-vpn tcp-tuning disable` — только принадлежащие службе
маршруты и её список. BBR действует на новые TCP-соединения, не UDP-туннель,
и не гарантирует скорость; возможна цена в повторных передачах.

Проверки: `make test-go`, `shellcheck -e SC2029 vpn/*.sh`.
Справка: [URLTest](https://sing-box.sagernet.org/configuration/outbound/urltest/),
[Clash API](https://sing-box.sagernet.org/configuration/experimental/clash-api/).
