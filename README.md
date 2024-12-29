# dots

Этот репозиторий содержит мои личные конфигурации NixOS и Home Manager.

## Текущая конфигурация

- **Оконный менеджер**: Hyprland
- **Терминал**: Kitty
- **Оболочка**: ZSH
- **Редактор**: Neovim
- **Дисплейный менеджер**: Emptty
- **Аудио**: PipeWire
- **Браузер**: Google Chrome

## Установка

### Установка NixOS

1. Загружаемся с официального ISO образа NixOS (minimal)

2. Подключаемся к интернету (если не подключились автоматически):
```bash
sudo systemctl start NetworkManager
sudo nmtui
```

3. Разметка дисков (пример с EFI):
```bash
sudo gdisk /dev/nvme0n1  # или ваш диск
# Создать:
# - EFI раздел (тип ef00, 512M)
# - Root раздел (тип 8300, остальное место)

sudo mkfs.fat -F 32 /dev/nvme0n1p1
sudo mkfs.ext4 /dev/nvme0n1p2

sudo mount /dev/nvme0n1p2 /mnt
sudo mkdir -p /mnt/boot
sudo mount /dev/nvme0n1p1 /mnt/boot
```

4. Включаем flakes:
```bash
sudo nix-env -iA nixos.nixFlakes
mkdir -p ~/.config/nix
echo "experimental-features = nix-command flakes" | sudo tee ~/.config/nix/nix.conf
```

5. Клонируем репозиторий:
```bash
nix-shell -p git
cd /mnt
git clone https://github.com/hikaary/dotfiles.git
```

6. Генерируем и копируем конфигурацию оборудования:
```bash
sudo nixos-generate-config --root /mnt
cp /mnt/etc/nixos/hardware-configuration.nix /mnt/dotfiles/nixos/
```
> **Примечание**: Этот шаг заменит существующий hardware-configuration.nix в репозитории на новый, сгенерированный для вашего реального оборудования. Это необходимо, так как конфигурация оборудования специфична для каждой системы.

7. Устанавливаем NixOS:
```bash
sudo nixos-install --flake /mnt/dotfiles/nixos#
```

8. После перезагрузки:
```bash
git clone https://github.com/hikaary/dotfiles.git
cd dotfiles
home-manager switch --flake .#hikary
```

## Работа с конфигурациями

В репозитории есть две отдельные конфигурации Flake:
- В корневой директории (`flake.nix`) - конфигурация Home Manager для пользовательских программ и настроек
- В директории `nixos/` (`nixos/flake.nix`) - конфигурация NixOS для системных настроек и программ

### Добавление новых программ

#### Пользовательские программы (Home Manager)
Для программ, которые нужны только вашему пользователю (например, браузер, терминал, текстовый редактор):

1. Добавьте программу в `home-manager/home.nix`:
```nix
{
  home.packages = with pkgs; [
    your-new-package  # Добавьте сюда
  ];
}
```

2. Если программа требует конфигурации, создайте для неё модуль в `home-manager/programs/`:
```bash
touch home-manager/programs/your-program.nix
```

3. Примените изменения:
```bash
cd ~/dotfiles
home-manager switch --flake .#hikary
```

#### Системные программы (NixOS)
Для системных программ или настроек (например, драйверы, системные сервисы, демоны):

1. Добавьте программу в `nixos/default.nix`:
```nix
{
  environment.systemPackages = with pkgs; [
    your-system-package  # Добавьте сюда
  ];
}
```

2. Примените изменения:
```bash
cd ~/dotfiles/nixos
sudo nixos-rebuild switch --flake .#
```

### Когда что использовать

- **Home Manager** (корневой `flake.nix`):
  - Пользовательские программы (браузеры, редакторы, IDE)
  - Конфигурации программ (dotfiles)
  - Темы и шрифты
  - Настройки окружения пользователя
  - Всё, что не требует root прав

- **NixOS** (`nixos/flake.nix`):
  - Системные программы и сервисы
  - Драйверы
  - Настройки загрузчика
  - Сетевые настройки
  - Всё, что требует root прав

## Обновление системы

### Обновление NixOS:
```bash
cd ~/dotfiles/nixos
sudo nixos-rebuild switch --flake .#
```

### Обновление Home Manager:
```bash
cd ~/dotfiles
home-manager switch --flake .#hikary
```

### Обновление зависимостей Flake:
```bash
# Для NixOS
cd ~/dotfiles/nixos
nix flake update

# Для Home Manager
cd ~/dotfiles
nix flake update
```

## Структура
```
.
├── flake.nix           # Конфигурация Flake для Home Manager
├── flake.lock          # Зафиксированные зависимости Home Manager
├── home-manager/       # Конфигурации Home Manager
│   ├── home.nix       # Основная конфигурация home-manager
│   └── programs/      # Конфигурации отдельных программ
├── nixos/             # Системные конфигурации NixOS
│   ├── flake.nix     # Конфигурация Flake для NixOS
│   ├── flake.lock    # Зафиксированные зависимости NixOS
│   ├── default.nix   # Основная системная конфигурация
│   └── hardware-configuration.nix  # Конфигурация оборудования
└── README.md          # Этот файл
