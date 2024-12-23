#!/bin/bash

# Устанавливаем только Nix и базовые системные утилиты
sudo pacman -S --needed \
	nix \
	reflector \
	xf86-input-wacom \
	pipewire \
	usbutils

mkdir -p ~/.config/nix

cat >~/.config/nix/nix.conf <<EOF
experimental-features = nix-command flakes

# Оптимизации сборки
max-jobs = auto
cores = 0
EOF

sudo tee /etc/nix/nix.conf >/dev/null <<EOF
#
# https://nixos.org/manual/nix/stable/#sec-conf-file
#

# Unix group containing the Nix build user accounts
build-users-group = nixbld

# Build optimizations
builders-use-substitutes = true
narinfo-cache-negative-ttl = 0

# Binary cache
substituters = https://cache.nixos.org
trusted-public-keys = cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY=
EOF

sudo systemctl restart nix-daemon

# Добавляем home-manager в NIX_PATH
export NIX_PATH=$HOME/.nix-defexpr/channels:/nix/var/nix/profiles/per-user/root/channels${NIX_PATH:+:$NIX_PATH}
nix-channel --add https://github.com/nix-community/home-manager/archive/master.tar.gz home-manager
nix-channel --update

home-manager switch --flake .#hikary

# Включаем сервисы pipewire
systemctl --user enable --now pipewire.service
systemctl --user enable --now pipewire-pulse.service

echo "Installation complete! Please restart your terminal."
