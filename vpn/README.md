# VPN-шлюз

AmneziaWG между Keenetic/Netcraze с HTTP RCI и Debian/Ubuntu VPS; sing-box разделяет
маршруты. Нужны SSH по ключу, прямой доступ к роутеру, локально Bash, SSH/SCP, curl и Go 1.26+.

```sh
./scripts/build-go-tools.sh
./vpn/install.sh --help
./vpn/install.sh
```

Установщик запрашивает секреты и **меняет VPS/роутер**: проверьте цели и сохраните бэкапы.
`--skip-router` пропускает роутер; `--skip-vps` сохраняет сервер и существующих пиров,
подходит для второго дома. Не повторяйте полную установку вслепую после обрыва SSH.
[Секреты и перенос](../README.md#приватные-настройки) — вне Git.
Обновление репозитория само не обновляет VPS.

## Эксплуатация

- На VPS: helper `/etc/sing-box/dots-vpn`, настройки `/etc/sing-box/vpn.env`,
  секрет панели `/etc/sing-box/secret.txt`, пиры `/root/awgvpn/peers/`.
- Службы: `systemctl status sing-box awg-quick@awg0 awg-gateway`.
  `vpn-health.timer` проверяет HTTPS через proxy без direct fallback/перезапусков;
  результат — `journalctl -u vpn-health.service`. Это не проверка Wi-Fi, proxy-dns
  или всей цепочки LAN. Диагностический `HEALTH_PORT=7896` слушает localhost.
- Панель: `http://10.8.2.1:9090/ui/` через туннель, пароль обязателен. Снаружи —
  `ssh -N -L 9090:127.0.0.1:9090 root@VPS` и localhost URL. Не открывайте HTTP-панель
  в недоверенную сеть. MTU — 1280.
- `AWG_LAN` назначает выбранному сегменту туннель; другие сегменты не меняются,
  индивидуальные правила приоритетнее. WAN fallback/IPv6/утечки проверяются отдельно;
  старые сторонние хуки автоматически не удаляются.
- Ручной selector сохраняет все поддерживаемые узлы. Auto — до 8 уникальных,
  без LTE/3G/4G и «Обход БС». В `vpn.env`: `AUTO_INCLUDE`/`AUTO_EXCLUDE` — regex
  без учёта регистра, `AUTO_MAX` — лимит; пустой `AUTO_EXCLUDE=` отключает фильтр.
  `AUTO_PREFER` ставит совпавшие имена перед дедупликацией/лимитом, сохраняя порядок
  внутри групп. Параметры сохраняются при переустановке; невалидный/пустой пул
  не заменяет рабочий конфиг. Лимит проверок не обещает максимальной скорости.

## Измерения на VPS

Нужны sing-box и curl. Helpers используют отдельный авторизованный localhost proxy,
очищают свои процессы/файлы и не меняют рабочие selector, службы или firewall.

```sh
umask 077
VPN=/etc/sing-box/dots-vpn
VPN_BENCH_CONFIG=$(mktemp)
trap 'rm -f "$VPN_BENCH_CONFIG"' EXIT
"$VPN" nodes --output "$VPN_BENCH_CONFIG"    # свежая подписка, без изменения gateway
"$VPN" benchmark --config "$VPN_BENCH_CONFIG" --all
"$VPN" benchmark --config "$VPN_BENCH_CONFIG" --only 'точное имя узла' \
  --samples 3 --bytes 10485760 --timeout 20
```

Поддерживаются VLESS и Hysteria2/hy2 с TLS/SNI/obfs из URI. `--all` проверяет узлы
переданного файла, не скачивает подписку. Сопоставьте число строк и причины пропусков.
XHTTP sing-box здесь не поддерживает: индекс пропуска выводится явно. Для него:

```sh
"$VPN" xhttp-benchmark --source-index "$XHTTP_INDEX" --check-only
"$VPN" xhttp-benchmark --source-index "$XHTTP_INDEX" --samples 3 --max-duration 240
```

Индекс — с нуля по свежей сырой подписке, включая пустые строки; при перестановке
строк проверьте его заново. XHTTP сохраняется, не подменяется TCP и не добавляется в auto.
На Linux amd64 скачивается закреплённый Xray с SHA256-проверкой; `--install-only` лишь
готовит его, `--check-only` не проверяет соединение. Failed/skipped дают exit 1.
Аргументы, размеры проб, URL и бюджеты — в `benchmark --help`/`xhttp-benchmark --help`.
Это VPS→узел→источник, не LAN/Wi-Fi/AWG: финалистов измеряйте отдельно из дома.
Один удачный пик не доказывает общего ускорения; секреты в URL не используйте.

## TCP tuning и откат

После замера: `/etc/sing-box/dots-vpn tcp-tuning install --peer 10.8.2.2/32`.
IP должен совпадать с AllowedIP пира `awg0`; повторяемые `--peer` задают полный список.
Без флага сохраняется `/etc/awg-tcp-tuning.json`, новый список пуст. Служба восстанавливает
свои маршруты после AWG restart; установка AWG не перезапускает. Глобальные Cubic/qdisc
и остальные пиры не меняются. Откат: `/usr/local/libexec/dots-vpn tcp-tuning disable` —
только маршруты службы и её список. BBR действует на новые TCP-соединения, не UDP-туннель;
скорость не гарантирует и может увеличить повторные передачи.

Проверки: `make test-go`, `shellcheck -e SC2029 vpn/*.sh`.
