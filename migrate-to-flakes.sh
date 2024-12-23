#!/bin/bash

# Enable flakes
mkdir -p ~/.config/nix
echo "experimental-features = nix-command flakes" >~/.config/nix/nix.conf

home-manager switch --flake .#hikary
