#!/bin/bash

# Install system packages
sudo pacman -S --needed \
	nix \
	hyprland \
	xdg-desktop-portal-hyprland \
	xdg-desktop-portal \
	xdg-utils \
	reflector \
	xf86-input-wacom \
	cpupower \
	networkmanager \
	pipewire \
	rsync \
	nvim \
	wireplumber \
	usbutils

# Enable flakes
mkdir -p ~/.config/nix
echo "experimental-features = nix-command flakes" >~/.config/nix/nix.conf

# Setup home-manager with flakes
ln -sf ~/dotfiles/home-manager ~/.config/home-manager

nix run home-manager/master -- init --switch
nix build .#homeConfigurations.hikary.activationPackage
./result/activate

# Enable services
systemctl --user enable --now pipewire.service
systemctl --user enable --now pipewire-pulse.service
