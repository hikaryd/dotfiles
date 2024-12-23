#!/bin/bash

# Enable flakes
mkdir -p ~/.config/nix
echo "experimental-features = nix-command flakes" > ~/.config/nix/nix.conf

# Remove old channels
nix-channel --remove home-manager
nix-channel --remove catppuccin

# Build and activate new configuration
cd ~/dotfiles
nix build .#homeConfigurations.hikary.activationPackage
./result/activate
