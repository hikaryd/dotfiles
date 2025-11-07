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

Изменения применятся мгновенно через симлинки!

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

# Только macOS настройки
./install -c steps/macos.yml
```

Настройки macos
```
./scripts/macos-defaults.sh
```

