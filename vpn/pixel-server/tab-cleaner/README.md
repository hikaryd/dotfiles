# Самоочистка вкладок Pixel: 12 часов

## Разовая разрешённая очистка 2026-09-12

По отдельному запросу владельца закрыты 24 старые фоновые вкладки из 45;
21 оставлена. Для закрытия требовались возраст наблюдения и навигации >=1 часа,
отсутствие подключённого CDP-клиента до/после проверки, скрытая страница без
фокуса, обнаруженных изменений полей, editable-текста, играющих media и
`onbeforeunload`. Перед закрытием повторно сверялись target ID, URL и attached.
Reddit, Google Docs, вкладки с меткой закрепления Hermes и исходно attached
targets исключались. При таймауте/неясном состоянии вкладка оставлялась.

Это консервативные наблюдаемые признаки, не доказательство отсутствия любой
будущей задачи/любого несохранённого состояния. Сохранность защищённых targets
и побайтная сохранность Reddit cookies проверены; cookie values не сохранялись.
RAM после очистки: около 1.8–1.9 GiB MemAvailable вместо примерно 1 GiB.
Разовая операция **не меняла** постоянный 12-часовой timer и его семантику ниже.

Для Hermes добавлена инструкция [доступа к Pixel](../HERMES-ACCESS.md),
установленная как `/root/hermes-ops/PIXEL-ACCESS.md` и связанная из `SOUL.md`.

Независимый stdlib Python oneshot запускается **на VPS host**, вне Hermes/Incus.
Он использует существующий приватный CDP tunnel `127.0.0.1:19222`.
Перезапускать Hermes, браузерный мост или телефон для установки не требуется.

## Точная семантика

- Все CDP targets с `type=page`, включая активную вкладку, закрываются при
  возрасте **не менее 43 200 секунд**. Это может прервать текущую работу/форму.
- У уже открытых вкладок возраст начинается с **первого успешного наблюдения**
  после установки: CDP не сообщает надёжное время их создания.
- Проверка каждые пять минут: обычно закрытие через 12 часов — 12 часов 5 минут
  от первого наблюдения (плюс задержки планировщика). Недоступность туннеля
  откладывает закрытие до успешной проверки.
- Навигация, обновление страницы и перезапуск скрипта **не сбрасывают возраст**.
  Пропавший из успешного снимка target забывается; новый ID получает возраст 0.
- Используются Linux boot ID и монотонные часы: перевод системного времени не
  влияет на возраст. После перезагрузки VPS сохранённый возраст остаётся,
  неизвестное время простоя не добавляется. При откате монотонных часов тоже
  не добавляется неизвестный интервал.
- Если state повреждён/отсутствует, все текущие вкладки начинают с нуля;
  массового закрытия из-за повреждения не происходит. Ошибка/невалидный CDP
  список не изменяет state и не закрывает вкладки. Неудачное закрытие повторится.
- В state записываются только ID и возраст, boot ID и монотонная отметка.
  URL, заголовки, cookies не сохраняются; логи содержат только счётчики.
- State атомарный, 0600, каталог 0700; параллельные запуски исключает flock.
  Ответ CDP ограничен 1 MiB, сетевой timeout 5 секунд. HTTP redirects и прокси
  окружения не используются. Никаких новых слушающих портов.

## Установка (выполняет владелец deployment)

```sh
install -d -m 755 /usr/local/lib/pixel-tab-cleaner
install -m 644 tab_cleaner.py /usr/local/lib/pixel-tab-cleaner/
install -m 644 pixel-tab-cleaner.service pixel-tab-cleaner.timer /etc/systemd/system/
systemd-analyze verify /etc/systemd/system/pixel-tab-cleaner.{service,timer}
systemctl daemon-reload
systemctl enable --now pixel-tab-cleaner.timer
systemctl start pixel-tab-cleaner.service
systemctl status pixel-tab-cleaner.timer pixel-tab-cleaner.service
journalctl -u pixel-tab-cleaner.service -n 10 --no-pager
```

Service использует DynamicUser, private StateDirectory, MemoryMax=64M, no
capabilities, read-only host filesystem, закрытый home, только IPv4 loopback
network. `IPAddressDeny/Allow` требуют поддержки systemd cgroup BPF на VPS;
проверить `systemd-analyze security` и journal, не считать unit-файл доказательством
runtime enforcement. Скрипт сам обращается только к literal 127.0.0.1:19222.

Проверки: `python3 -m unittest discover -s . -p 'test_*.py' -v`.
Для изолированной E2E проверки доступен `--state-dir /private/test-directory`;
TTL и CDP endpoint намеренно не переопределяются. Подготавливать aged state
только для собственной тестовой вкладки, не менять production state.

Откат: `systemctl disable --now pixel-tab-cleaner.timer`, затем
`systemctl stop pixel-tab-cleaner.service`. Это не затрагивает Hermes или Chrome.
