# Docker: не развёрнут, доступ Hermes не выдан

> Переносимый runbook: адреса TEST-NET, имена и UID ниже — примеры,
> не описание вашей установки. Актуальные endpoints/ключи/результаты проверок
> храните в [приватном inventory](PRIVATE-CONFIG.md), не в Git.


Проверка Pixel 8 / Android 16 CP11.251209.007.A1, 2026-09-12.
Браузерный мост и owner noVNC — отдельные работающие компоненты, не Docker.

## Дополнение: постоянный SSH, 2026-09-12 ~17:38 UTC

Постоянный shell Termux владельца теперь настроен отдельно от Docker:
[SSH runbook](ssh-owner/README.md). Он не даёт Hermes root/Docker/owner keys.
Свободно около 82 GiB **диска**, но MemAvailable в последних замерах лишь
0.95–1.3 GiB, свободный swap около 0.7–0.75 GiB. Это разные ресурсы: свободный диск
не разрешает безопасно выделить ещё 1.5–2 GiB оперативной памяти VM.
Повторный VM boot при таком запасе не запускался. Пользователь выбрал
автоочистку вкладок через 12 часов, поэтому нестарые рабочие вкладки для
освобождения RAM принудительно не закрывались. Docker по-прежнему не установлен.
Ранее загруженный проверенный Debian kernel — заготовка, не установленное
или успешно загруженное гостевое ядро. Нужно пройти оставшиеся gates ниже.

## Обновление после разрешённой очистки, 2026-09-12 ~16:00 UTC

- Отключены 53 обычных приложения через `pm disable-user`, **без удаления
  данных**. Исходные enabled states сохранены в приватном
  `~/.local/share/hermes-pixel/docker-prototype/evidence/disabled-apps-backup.json`.
  Chrome, Termux, droidVNC, VPN/proxy, password manager и системные компоненты
  не отключали. Android подтверждает AC charging.
- После закрытия cached processes пять замеров показали около 2.8 GiB
  MemAvailable и около 2.5 GiB SwapFree. Это позволило первый bounded probe.
- `AvfProbe preflight` **реально прошёл на Pixel**: 512 MiB, один vCPU,
  network=false, shares=0, balloon=false. 512 MiB — диагностический объём,
  не обещанный окончательный бюджет Docker.
- Custom AVF **реально загрузила Debian 13.2**, kernel
  `6.12.60-android16-6-g54e1389bda83-ab14631638-4k`, CID 2048.
  `cloud-final` завершился, key-only SSH/VSOCK units стартовали. Вопреки прежним
  выводам по policy/source, console output на этой сборке фактически доступен
  с текущей конфигурацией debug NONE. Никакие Android debug/SELinux policies
  не ослабляли.
- Прямой `adb forward vsock:2048:22022` запрещён самим adbd (для guest CID
  разрешён только порт 5555). Ограничение не обходили. Подготовлен отдельный
  bounded relay через разрешённый `vm.connectVsock(22022)`; runtime SSH через
  новый relay пока не проверен.
- На 104-й секунде монитор остановил **только новую VM** из-за роста swap.
  Независимый `vm list` подтвердил пустой список; основные PID Chrome,
  Termux, VNC, bridge/helper/Caddy сохранились. Общий compatibility gate ещё
  **не пройден**. В Chrome одновременно обнаружены 36 вкладок; они не закрыты
  без уточнения, не относятся ли к текущей работе Hermes.
- Из реально загруженного kernel Image извлечён embedded config:
  `CONFIG_PID_NS` и `CONFIG_CGROUP_PIDS` **не включены**. Поэтому штатное
  гостевое ядро не подходит для обычного Docker с требуемой изоляцией и
  PID-лимитами. Нужны стандартное гостевое Debian kernel + matching modules
  и initramfs; менять/прошивать ядро Android для этого не требуется.

Доказательства находятся в приватном `docker-prototype/evidence/`:
`admission.json`, `boot-1.log`, `boot-1.json`, `guest-kernel.config`,
`staged-sha256.txt`. Исторический hash seed в `staged-sha256.txt` относится к
версии до финального SSH hardening; актуальный seed надо хешировать отдельно.
Source-only probe и guardrail tests: `vpn/pixel-server/avf-probe/`.
Ни постоянная VM, ни Docker, ни новые права Hermes ещё не включены.

## Что не прошло входной security gate

Стандартный Google Terminal image tag `4000000-bug451076949` содержит
`memory_mib=4096` и shared paths `/storage/emulated` и `$APP_DATA_DIR/files`.
Terminal отображает первый путь в Android Downloads. Нельзя выдавать гостю с
Docker/агентом личные файлы телефона под видом изолированного сервера.
На момент проверки MemAvailable было около 2.1 ГБ: запуск VM 4 ГБ не проверяли.
Конфиг извлечён до запуска; SHA256
`d77acbdf315999e9ec9f4454e74d68d8f87fe717119d3b13513c99ba4aadfada`.
VM не запускалась, PIN/root/bootloader/прошивка не менялись.

## Уточнение custom AVF

У `com.android.shell` реально выданы `USE_CUSTOM_VIRTUAL_MACHINE`,
`MANAGE_VIRTUAL_MACHINE`, `DEBUG_VIRTUAL_MACHINE`, `INTERNET`.
Поэтому утверждение «custom VM запрещена просто потому, что user-build» неверно.
Raw `vm run` с 1 ГБ и без shared paths возможен как ещё **не проверенный boot**.
Но в ближайшей публичной ветке Android16 QPR2 raw command не подключает сеть;
показанный help флаг `--network-supported` сам по себе этого не доказывает.
Исходник не тождественен установленному QPR3 beta: сетевой путь на конкретной
сборке через raw CLI не доказан. Это не готовая замена штатному сетевому Terminal VM.

### Повторная проверка после физического отключения USB, 15:32 UTC

Пользователь подтвердил отключение кабеля от компьютера. Запрос CDP из самого
Incus `hermes-agent` по `127.0.0.1:19222/json/version` возвращает настоящий
Android Chrome 145. Gateway остаётся active, MainPID `<gateway-pid>`, время старта
`2026-09-12 11:50:59 UTC`; Hermes не перезапускался.

На **установленной** QPR3 framework-библиотеке найден другой путь:
`VirtualMachineCustomImageConfig.Builder.useNetwork(boolean)` передаётся в
`VirtualMachineRawConfig.networkSupported`, а список host shares изначально
пустой. Shell уже имеет необходимые AVF permissions. Это подтверждает наличие
API для custom VM без host shares, **не успешный запуск VM или Docker**.
SHA256 `/apex/com.android.virt/javalib/framework-virtualization.jar`:
`424df09c99390149961102b1201b4901462a598b90c84fba9671a22e218a0ebe`.

Будущий helper через `app_process` требует shell-owned Context/data directory,
public `VirtualMachineConfig.Builder(Context)` и живого Binder-клиента весь
срок VM. Перегрузка Builder(String) private и для обычного Java-клиента не
подходит. Это нестандартная SystemApi-интеграция, а не готовая штатная установка.
Без сети `connectVsock()` требует заранее установленного гостевого listener;
обычный TCP sshd таким listener не является. Работа guest networking,
bootstrap, cgroups, overlayfs, Docker и автономного supervisor ещё не проверена.

**Текущий ресурсный стоп:** MemAvailable `1264904 kB` (~1.21 GiB),
SwapFree `810232 kB` из `3877328 kB` (~79% swap занято). Даже гостевые 512 MiB
не оставляют предусмотренный планом 1 GiB резерва Android с учётом overhead.
AC/USB/wireless charging на момент снимка — false; батарея 100%, 28°C.
`vm list` возвращает пустой список. Не закрывали пользовательские приложения,
не запускали VM, не устанавливали Docker и не выдавали новые права Hermes.

QEMU/TCG внутри инфраструктурного Termux не принят как замена: согласно
[security policy QEMU](https://www.qemu.org/docs/master/system/security.html),
TCG не предоставляет гарантии guest isolation. Его побег наследовал бы UID с
ключами браузерного моста. Отсутствие shares само по себе не разделяет этот UID.

Локальный снимок: `~/.local/share/hermes-pixel/docker-prototype/evidence/preflight.txt`.
Загрузка Google guest image остановлена до установки и сохранена как
`images.tar.gz.partial`; это неполный, непроверенный архив, запускать его нельзя.
Следующий boot допустим только после восстановления запаса памяти и питания,
с сохранением исходных isolation/resource gates. Освобождение RAM само по себе
не является доказательством работоспособности Docker.

- [Точная исследованная raw command implementation](https://android.googlesource.com/platform/packages/modules/Virtualization/+/739926533e4141ec943a462b39583328110fe4ef/android/vm/src/run.rs#283)
- [Raw config schema](https://android.googlesource.com/platform/packages/modules/Virtualization/+/739926533e4141ec943a462b39583328110fe4ef/libs/vmconfig/src/lib.rs#40)
- [Официальный custom VM workflow](https://android.googlesource.com/platform/packages/modules/Virtualization/+/refs/heads/main/docs/custom_vm.md)

## Перед будущим предоставлением прав

Нужны одновременно: реальный Linux kernel/Docker daemon, отсутствие личных
host shares и секретов, ограничение памяти/диска, проверенная сеть с отдельными
allowlist-правами для конкретных домашних адресов/портов, отдельный ограниченный
диспетчер команд Hermes (не owner noVNC/ADB/root/Docker socket), rollback и soak.
Не открывать Docker API на LAN/WAN и не считать PRoot изоляцией контейнерного
ядра. Пока эти условия не доказаны, Hermes получает только прежний браузерный
маршрут; права на Docker/всю домашнюю сеть автоматически не добавляются.
