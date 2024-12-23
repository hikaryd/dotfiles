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

1. Загружаемся с установочного носителя NixOS

2. Подключаемся к интернету
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

4. Генерируем начальную конфигурацию:
```bash
sudo nixos-generate-config --root /mnt
```

5. Клонируем репозиторий:
```bash
sudo nix-env -iA nixos.git
cd /mnt
git clone https://github.com/hikaary/dotfiles.git
```

6. Копируем конфигурацию оборудования:
```bash
cp /mnt/etc/nixos/hardware-configuration.nix /mnt/dotfiles/nixos/
```

7. Устанавливаем NixOS:
```bash
sudo nixos-install --flake /mnt/dotfiles#nixos
```

8. После перезагрузки:
```bash
git clone https://github.com/hikaary/dotfiles.git
cd dotfiles

home-manager switch --flake .#hikary
```

## Обновление системы

### Обновление NixOS:
```bash
sudo nixos-rebuild switch --flake .#nixos
```

### Обновление Home Manager:
```bash
home-manager switch --flake .#hikary
```

### Обновление зависимостей Flake:
```bash
nix flake update
```

## Структура
```
.
├── flake.nix           # Конфигурация Flake
├── flake.lock          # Зафиксированные зависимости
├── home-manager/       # Конфигурации Home Manager
│   ├── home.nix       # Основная конфигурация home-manager
│   └── programs/      # Конфигурации отдельных программ
├── nixos/             # Системные конфигурации NixOS
│   ├── configuration.nix     # Основная системная конфигурация
│   └── hardware-configuration.nix  # Конфигурация оборудования
└── README.md          # Этот файл
