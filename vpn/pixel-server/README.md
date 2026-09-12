# Pixel: защищённое управление экраном из LAN

> Переносимый runbook: адреса TEST-NET, имена и UID ниже — примеры,
> не описание вашей установки. Актуальные endpoints/ключи/результаты проверок
> храните в [приватном inventory](PRIVATE-CONFIG.md), не в Git.


На Pixel развёрнуто и проверено 2026-09-12: owner-режим, noVNC и HTTPS gateway.
Docker **не установлен**: см. [отдельный статус](DOCKER-STATUS.md).
Адрес владельца: **https://192.0.2.116:8443/**.
Адрес закреплён на роутере: [DHCP-резервация](DHCP.md).
Терминал, пакеты и свои сервисы: [SSH владельца](ssh-owner/README.md).
`https://192.0.2.116/` использует другой порт (443) и здесь не обслуживается.

## Схема и предварительные условия

- droidVNC-NG **2.21.0**: `interfaceName=lo`, VNC `5900`, bundled noVNC HTTP
  `5800`, отдельный VNC-пароль, `fileTransfer=false`, `startOnBoot=false` до
  проверки жизненного цикла. Сначала конфигурация привязки, затем запуск.
- Пакетный Caddy **2.11.4** без плагинов в Termux: TLS + отдельная Basic auth
  владельца, единственный listener на указанном IPv4:8443, без HTTP/QUIC/admin API.
- noVNC показывает Android и передаёт разрешённый владельцем ввод. Это не
  терминал, не Docker UI и не способ обойти PIN/защищённые окна Android.
- Hermes не получает пароль владельца/VNC, ключи Termux/ADB или Docker socket.
  Его текущий браузерный туннель 19222 остаётся отдельным. Hermes не перезапускать.

## Подстановка шаблона

Сформировать отдельный `$HOME/pixel-admin/Caddyfile` в Termux, права каталога
`700`, конфигурации и приватных файлов `600`. В репозиторий секреты не писать.
Подставлять точные строки безопасным скриптом замены, **не shell eval/envsubst**
(символы `$` в bcrypt должны сохраниться).

| Маркер | Значение |
| --- | --- |
| `__LAN_IP__` | `192.0.2.116`, во всех местах одновременно |
| `__TERMUX_HOME__` | `/data/data/com.termux/files/home` |
| `__OWNER_USERNAME__` | отдельный логин владельца без пробелов/спецсимволов |
| `__OWNER_BCRYPT_HASH__` | bcrypt-хеш сильного уникального пароля; не plaintext |
| `__CONTROL_TOKEN__` | отдельный сильный случайный токен helper, только в приватных Caddyfile и `control.json` на телефоне |

Сертификат `$HOME/pixel-admin/tls.crt` должен иметь SAN IP `192.0.2.116`;
ключ — `tls.key`. Пароли хранить в менеджере паролей владельца, не в URL,
истории команд, документации или окружении Hermes. При самоподписанном
сертификате предварительно сверить SHA-256 fingerprint по доверенному каналу:
браузеру потребуется явное доверие владельца. `curl -k` **не** доказывает
доверие сертификату. Не устанавливать системный CA молча.

Перед запуском на телефоне:

```sh
caddy version
caddy adapt --config "$HOME/pixel-admin/Caddyfile" --adapter caddyfile --validate >/dev/null
caddy validate --config "$HOME/pixel-admin/Caddyfile" --adapter caddyfile
```

После успешной валидации запускать только этот новый gateway; не перезагружать
Hermes, Incus, VPS или телефон. Admin API отключён, поэтому штатный API reload
не доступен: изменение gateway требует отдельного контролируемого перезапуска
только Caddy и разрывает его текущие VNC-сессии.

## Политика и проверки

Шаблон разрешает **104 точных runtime-пути** из 109 `assets/novnc/` файлов APK
2.21.0. Исключены Makefile, locale README, sounds CREDITS, vendor LICENSE/README.
Нет wildcard/static file server: `/defaults.json`, неизвестные маршруты,
неканонические пути, upload/CONNECT/TRACE запрещены. Исключение для POST — только
две owner-команды ниже. GET/HEAD статических ресурсов
идут только на `127.0.0.1:5800` (backend всегда получает GET: его HEAD не
поддерживается; для клиентского HEAD Caddy не отправляет тело); GET `/websockify` с WebSocket Upgrade и точным
Origin — только на `127.0.0.1:5900`. Authorization/Proxy-Authorization/Cookie
удаляются перед backend. Чужой Host отклоняется до auth; все разрешённые пути,
включая redirect и WebSocket, требуют TLS и Basic auth.
Site address `https://:8443` принимает любой Host только на точном `bind` IP,
чтобы проверка Host внутри route всегда выполнялась. Hostname в site address
здесь недопустим: чужой Host иначе может обходить route и получать пустой 200.

Обязательная runtime-проверка: HTML + живое изменяющееся изображение + ввод;
reconnect; работа при погашенном экране; сохранность браузерного моста Hermes;
отказ без/с неверным паролем на HTML и WS; отказ без/с чужим Origin на WS;
отказ чужому Host, POST, не-upgrade `/websockify`, обходам путей; недоступность
5800/5900/2019/80 с LAN и отсутствие wildcard/IPv6 listeners. Сертификат проверять
через `--cacert` с публичным leaf, затем отдельно реальным браузером владельца.

Bundled HTTP backend не выдаёт Content-Type для `/vnc.html`: шаблон явно
задаёт `text/html; charset=utf-8` **только этому allowlisted HTML-пути**,
сохраняя `nosniff`. Для точных allowlisted PNG/TTF/OGA/JSON путей также заданы
соответственно image/png, font/ttf, audio/ogg, application/json; wildcard MIME
правил нет. HTTP 200 недостаточен: проверять MIME и реальный DOM/UI.

CSP не разрешает inline/eval JavaScript; inline **styles** нужны upstream noVNC.
WebSocket не имеет короткого stream/write timeout; заголовки/начальный ответ
backend ограничены временем. Access logs выключены, runtime logging отправлен
в `discard`, чтобы ошибки не сохраняли URL/заголовки. Это сознательно ограничивает
последующую диагностику. В шаблоне нет защиты от перебора/DoS по числу попыток:
сильный случайный пароль и доверенная LAN обязательны; HTTP timeouts не являются
rate limiting.

## Временный режим управления экраном

Корень перенаправляет на `/owner`: GET-страница с фиксированными HTML-формами,
без JavaScript. «Начать» включает owner lease на 15 минут; «Остановить» завершает
его. Успешный start перенаправляет прямо в noVNC. Это интерфейс ограниченного helper,
не произвольное выполнение команд. Реальный start → мышь → reconnect → stop
проверен; после stop Chrome/CDP восстановились, панель OFF.

Gateway передаёт GET `/owner`, GET `/owner/status` и POST `/owner/start`, `/owner/stop` без query
на `127.0.0.1:8766`. POST требует точный `Origin: https://192.0.2.116:8443`;
Host, Basic auth и каноничность пути проверяются **до всех** owner-маршрутов.
Для helper принудительно задаются `Host: 127.0.0.1:8766` и серверный
`X-Pixel-Control`; клиентский токен перезаписывается. Credentials/cookies не
передаются. Helper обязан самостоятельно проверять Host/token/path/method и
слушать только loopback. Токен не показывать браузеру, не передавать Hermes и
не публиковать в репозитории. Только `/owner` разрешает CSP `form-action 'self'`;
остальные страницы сохраняют запрет форм. Некорректный POST не достигает helper.
`/owner/status` только читает lease: GET не будит экран и не продлевает управление.
`owner-session.js` проверяет lease до загрузки штатного `/app/ui.js`, затем
каждые 5 секунд. Вне lease или при ошибке возвращает `/owner` вместо старого
кадра. При деплое нужны `~/pixel-admin/static/vnc.html` и `owner-session.js`:
в vendor HTML штатный module entrypoint удалён, добавлен этот guard, исходный
HTML скрыт до проверки. Остальные vendor-модули noVNC сохранены. Повторная
установка не должна возвращать безусловный auto-connect старого vendor HTML.
`Referrer-Policy: same-origin` сохраняет корректный Origin для POST HTML-форм
в пределах панели и не передаёт Referer на другой origin. `no-referrer` здесь
вызывал `Origin: null` в реальном браузере; проверку точного Origin не ослабляем.

## Ограничения безопасности и эксплуатации

- Caddy и браузерный мост работают под одним Android UID Termux. Компрометация
  Caddy потенциально раскрывает ключи моста; `chmod` не изолирует процессы одного
  UID. Шаблон минимизирует поверхность, но не устраняет этот остаточный риск.
- VNC-доступ владельца привилегирован и включает clipboard. `fileTransfer=false`
  **не отключает clipboard** в droidVNC-NG 2.21.0.
- Привязка к LAN IP — не firewall по адресу клиента. Не пробрасывать 8443 на WAN;
  при недоверенных маршрутизируемых сетях нужен отдельный ingress-контроль.
- Если DHCP поменяет IP, старый bind не обеспечивает доступ по новому адресу:
  сверить адрес, обновить весь шаблон и сертификат; не заменять bind на wildcard.
- Перезапуск Android/capture и длительная автономность требуют отдельных проверок
  и могут требовать ручного PIN/разрешения захвата. Docker/VM, ограниченные
  полномочия Hermes и доступ к конкретным внутренним устройствам — отдельный этап.
- Российские сервисы остаются без дополнительных прокси; настройки WG/роутера
  этот каталог не меняет.

## Официальные источники

- [droidVNC-NG 2.21.0](https://github.com/bk138/droidVNC-NG/tree/v2.21.0)
- [Caddy: matchers и CEL](https://caddyserver.com/docs/caddyfile/matchers)
- [Caddy: placeholders](https://caddyserver.com/docs/caddyfile/concepts#placeholders)
- [Caddy: global options](https://caddyserver.com/docs/caddyfile/options)
- [Caddy: Basic auth](https://caddyserver.com/docs/caddyfile/directives/basic_auth)
- [Caddy: reverse proxy](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy)


## Подтверждённое состояние и обслуживание (2026-09-12)

- 22 unit-теста PASS, в том числе 14 helper-тестов непосредственно в Termux;
  Ruff/форматирование, shellcheck, Python compile и Caddy validate PASS.
- Независимые 73 HTTP + 5 TCP проверки PASS: auth, Host/Origin, запрещённые
  методы/пути/query, MIME, отсутствие прямого LAN-доступа к backend/admin-портам.
  Ещё 6 прямых helper-negative проверок подтвердили token/Host/Origin/body/path.
- Реальный headed-браузер: HTML-форма start, noVNC, касание мышью Android Settings,
  reconnect, stop с возвратом Chrome и физическим OFF — PASS.
- Аварийный тест: helper убит, его supervisor приостановлен; тестовый lease
  сокращён до 20 секунд. Bridge сам вернул Chrome/CDP и OFF через 34.4 секунды
  от сбоя (включая опрос/handback). Затем supervisor/helper восстановлены.
  Это тест expiry-логики, **не** 15-минутный/12-часовой soak новой конфигурации.
- При OFF кнопка Esc/Back работает, но Android отвергает обычные pointer taps.
  Поэтому для мыши сначала «Начать 15 минут»: физическая панель станет видимой
  окружающим; завершение/expiry возвращает Chrome и OFF без PIN-блокировки.
- Во время owner lease браузерный туннель Hermes закрывается; сам агент остаётся
  запущенным. Переключение по polling, не мгновенное и не гарантия параллельной
  независимой работы владельца и агента с одним Chrome.
- На Mac данные входа находятся в защищённом локальном файле
  `~/.local/share/hermes-pixel/server-setup/owner-access.txt`, не в репозитории.
  HTTPS-пароль и VNC-пароль разные. Control-token браузеру/Hermes не выдаётся.
- Сертификат проверен по leaf/IP и в отдельном Chromium с точным SPKI pin.
  Доверие в пользовательском Waterfox **не устанавливалось**: первое исключение
  требует сверки fingerprint из локального файла, не глобального отключения TLS.
- Физический перенос кабеля на отдельную зарядку и reboot не проверялись.
  После reboot/PIN-lock может понадобиться ручная разблокировка и повторное
  согласие droidVNC-NG на захват. Обход PIN/защищённых окон не настроен.

Phone HOME: `/data/data/com.termux/files/home`. Production-файлы:
`pixel-admin/{owner_control.py,control.json,Caddyfile,tls.crt,tls.key}`,
`hermes-pixel/{bridge.py,display-owner-lease}`, boot scripts
`.config/termux/boot/{10-hermes-pixel,19-pixel-owner-control,20-pixel-admin}`.
Helper устанавливается **до** bridge, который импортирует его lease-логику.
Все три boot script в этом Google Play Termux dispatch-ятся асинхронно;
номер имени не означает readiness. В legacy `.termux/boot` дубликатов не найдено.

Runtime-диагностика без секретов: `pixel-admin/control-lifecycle.txt`,
`caddy-lifecycle.txt`, `control-last-error.json` (при ошибке),
`hermes-pixel/status.json` и `bridge.log`. Lifecycle-файлы ограничены последним
запуском/exit code; timestamp обязателен для оценки свежести, PID ≠ health.
Raw access/error logs не включать: они могут содержать чувствительные заголовки.
`caddy version` у этого Termux build пишет unknown; версию подтверждает
`dpkg-query -W caddy` = 2.11.4. droidVNC-NG исключён из battery optimization.

После изменения endpoint DHCP требуется обновить bind, все Origin/Host правила,
helper origin и сертификат; wildcard bind не использовать. Пробросов WAN нет.
Маршруты роутера/WG и политика РФ без дополнительных proxy не менялись.

Финальная проверка после удаления диагностического доступа Mac:
Hermes session `<session-id>` сам выбрал `browser_exec`, получил
`Example Domain` и Android Chrome UA; закрытие своей тестовой вкладки подтверждено
CDP. Gateway всё время `MainPID=<gateway-pid>`, active с `11:50:59 UTC` — без рестарта.
LAN `/owner` ответил 200 с проверенным TLS и после `adb disconnect` на Mac.
Временный SSH ключ отозван, setup sshd8022 остановлен, Mac forwards удалены;
это относится к первоначальной установке. Затем настроен отдельный постоянный
key-only [SSH владельца](ssh-owner/README.md) на порту 8022.
производственные каналы телефона остались активны. Физический кабель не проверялся.

Локальные доказательства: `~/.local/share/hermes-pixel/server-setup/evidence/`:
`owner-e2e.json`, `owner-crash-expiry.json`, `helper-negative.json`,
`gateway-security-final-v2.json`, `hermes-final-db-proof.json`,
`phone-final-before-teardown.txt`, `setup-teardown.txt`.
