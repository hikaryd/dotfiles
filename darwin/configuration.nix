{ pkgs, user, ... }: {
  nixpkgs.config.allowUnfree = true;
  nix.enable = false;

  environment.systemPackages = with pkgs; [ vim curl wget git tmux ];
  environment.shells = with pkgs; [ nushell ];

  users.users."${user}" = {
    home = "/Users/${user}";
    name = user;
    shell = pkgs.nushell;
  };

  security.pam.services.sudo_local.touchIdAuth = true;

  system.defaults = {
    NSGlobalDomain = {
      AppleShowAllExtensions = false;

      # 120, 90, 60, 30, 12, 6, 2
      KeyRepeat = 2;

      # 120, 94, 68, 35, 25, 15
      InitialKeyRepeat = 15;
    };

    dock = {
      autohide = true;
      show-recents = false;
      launchanim = true;
      mouse-over-hilite-stack = true;
      orientation = "bottom";
      tilesize = 48;
    };

    finder = {
      AppleShowAllExtensions = false;
      ShowPathbar = false;
      FXDefaultSearchScope = "SCcf";
    };
  };

  system.stateVersion = 4;
  homebrew = {
    enable = true;
    taps = [ "FelixKratz/formulae" ];
    brews = [ "spicetify-cli" ];
    casks = [
      "jordanbaird-ice"
      "ghostty"
      "chatgpt"
      "raycast"
      "hammerspoon"
      "1password-cli"
      "telegram"
      "zappy"
      "rectangle"
    ];
  };
}

