{ pkgs, ... }: {
  nixpkgs.config.allowUnfree = true;
  nix.enable = false;

  environment.systemPackages = with pkgs; [ vim curl wget git ];

  users.users."tronin.egor" = {
    home = "/Users/tronin.egor";
    name = "tronin.egor";
  };

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

  services.yabai = {
    enable = true;
    package = pkgs.yabai;
    enableScriptingAddition = true;
  };

  services.skhd = {
    enable = true;
    package = pkgs.skhd;
  };
}

