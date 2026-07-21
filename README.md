# Dotfiles

```bash
cd ~/dots
./install
```

## Установка с нуля

1. Установите Xcode Command Line Tools:
```bash
xcode-select --install
```

2. Установите Homebrew:
```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

3. Запустите установку:
```bash
./install
```

## Обновление конфигов

После изменения любых конфигов просто запустите:

```bash
cd ~/dots
./install
```

Большинство изменений применится через симлинки; смешанные конфиги Claude/Codex
обновляются их apply-шагами во время `./install`.

## Установка отдельных компонентов

Вы можете установить только определенные компоненты:

```bash
# Только brew пакеты
./install -c steps/dependencies.yml

# Только shell конфиги
./install -c steps/terminal.yml

# Только редакторы
./install -c steps/editors.yml

# Только CLI tools
./install -c steps/tools.yml

# Только личный конфиг Codex
./install -c steps/codex.yml

# Только macOS настройки
./install -c steps/macos.yml
```

Экспорт, секреты и правила владения Codex-настройками описаны в
[`config/codex/README.md`](config/codex/README.md).

Настройки macos
```
./scripts/macos-defaults.sh
```

## Откат установки

```bash
# посмотреть, что будет сделано (ничего не меняет)
./uninstall --dry-run

# откатить конфиги: симлинки, Firefox/Zen/AyuGram/Claude/Codex, твики шелла и defaults
./uninstall

# то же + удалить пакеты из Brewfile
./uninstall --packages
```

Удаляются только симлинки, указывающие в этот репозиторий. Скопированные файлы
переезжают в `~/dots-uninstall-backup-<дата>/`; смешанный Codex config сначала
копируется туда целиком, затем из оригинала снимаются только управляемые ключи.
Все действия перечисляются в `MANIFEST.txt` — ничего не теряется.

## Аудит лишнего

Что установлено в системе мимо Brewfile (кандидаты на чистку):

```bash
./scripts/audit-extras.sh
```
