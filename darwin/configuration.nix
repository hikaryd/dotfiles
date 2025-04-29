{ pkgs, ... }:

{
  nixpkgs.config.allowUnfree = true;

  environment.systemPackages = with pkgs; [ vim curl wget git kanata ];

  nix.gc = {
    automatic = true;
    interval.Day = 7;
    options = "--delete-older-than 30d";
  };

  nix.settings.auto-optimise-store = true;

  nix.settings.experimental-features = [ "nix-command" "flakes" ];
  security.pam.services.sudo_local.touchIdAuth = true;

  system.defaults = {
    NSGlobalDomain = {
      KeyRepeat = 2;
      InitialKeyRepeat = 15;
    };

    dock = {
      autohide = true;
      tilesize = 48;
      show-recents = false;
      magnification = true;
      largesize = 64;
    };

    finder = {
      AppleShowAllExtensions = true;
      ShowPathbar = true;
      FXDefaultSearchScope = "SCcf";
    };
  };
  system.stateVersion = 4;
}

