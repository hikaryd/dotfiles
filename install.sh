#!/bin/bash

sudo pacman -S --needed \
	nix \
	hyprland \
	xdg-desktop-portal-hyprland \
	xdg-desktop-portal \
	xdg-desktop-portal-gtk \
	xdg-utils \
	hyprlock \
	reflector \
	auto-cpufreq

ln -sf ~/dotfiles/home-manager ~/.config/home-manager

nix-collect-garbage -d &&
	nix-channel --add https://nixos.org/channels/nixpkgs-unstable nixpkgs &&
	nix-channel --add https://github.com/nix-community/home-manager/archive/master.tar.gz home-manager &&
	nix-channel --add https://github.com/catppuccin/nix/archive/main.tar.gz catppuccin &&
	nix-channel --update &&
	nix-shell '"'"'<home-manager>'"'"' -A install

sudo auto-cpufreq --install
systemctl --user enable --now pipewire.service
systemctl --user enable --now pipewire-pulse.service
