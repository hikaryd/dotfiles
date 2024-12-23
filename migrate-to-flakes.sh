#!/bin/bash

mkdir -p ~/.config/nix
echo "experimental-features = nix-command flakes" >~/.config/nix/nix.conf

nix-channel --remove home-manager
nix-channel --remove catppuccin

nix build .#homeConfigurations.hikary.activationPackage
./result/activate
