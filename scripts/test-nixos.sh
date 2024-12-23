#!/usr/bin/env bash

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${YELLOW}Начинаем тестирование NixOS конфигурации...${NC}"

# Проверка синтаксиса
echo -e "\n${YELLOW}1. Проверка синтаксиса конфигурации...${NC}"
if nix-instantiate --parse ./nixos/default.nix > /dev/null; then
    echo -e "${GREEN}✓ Синтаксис корректен${NC}"
else
    echo -e "${RED}✗ Ошибка в синтаксисе${NC}"
    exit 1
fi

# Проверка оценки конфигурации
echo -e "\n${YELLOW}2. Проверка оценки конфигурации...${NC}"
if nix-instantiate '<nixpkgs/nixos>' -A system -I nixos-config=./nixos/default.nix > /dev/null; then
    echo -e "${GREEN}✓ Конфигурация оценивается корректно${NC}"
else
    echo -e "${RED}✗ Ошибка в оценке конфигурации${NC}"
    exit 1
fi

# Пробная сборка
echo -e "\n${YELLOW}3. Пробная сборка системы...${NC}"
if nixos-rebuild build --flake .#nixos; then
    echo -e "${GREEN}✓ Система собирается успешно${NC}"
else
    echo -e "${RED}✗ Ошибка при сборке системы${NC}"
    exit 1
fi

# Создание образа QEMU (опционально)
echo -e "\n${YELLOW}4. Хотите создать образ QEMU для тестирования? [y/N]${NC}"
read -r response
if [[ "$response" =~ ^([yY][eE][sS]|[yY])+$ ]]; then
    echo -e "${YELLOW}Создаем образ QEMU...${NC}"
    if nix run nixpkgs#nixos-generators -- --format qcow -c ./nixos/default.nix; then
        echo -e "${GREEN}✓ Образ QEMU создан успешно${NC}"
        echo -e "${YELLOW}Запустить образ? [y/N]${NC}"
        read -r run_response
        if [[ "$run_response" =~ ^([yY][eE][sS]|[yY])+$ ]]; then
            qemu-system-x86_64 -enable-kvm -m 4G -smp 4 \
                -drive file=nixos.qcow2,if=virtio \
                -display gtk \
                -vga virtio \
                -device virtio-tablet \
                -device virtio-keyboard
        fi
    else
        echo -e "${RED}✗ Ошибка при создании образа QEMU${NC}"
    fi
fi

echo -e "\n${GREEN}Тестирование завершено!${NC}"
