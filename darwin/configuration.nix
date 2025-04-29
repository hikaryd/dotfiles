{ pkgs, user, ... }: {
  nixpkgs.config.allowUnfree = true;
  nix.enable = false;

  environment.systemPackages = with pkgs; [ vim curl wget git tmux ];

  users.users."${user}" = {
    home = "/Users/${user}";
    name = user;
    shell = { program = "${pkgs.nushell}/bin/nu"; };
  };

  security.pam.services.sudo_local.touchIdAuth = true;

  system.defaults = {
    NSGlobalDomain = {
      AppleShowAllExtensions = true;
      ApplePressAndHoldEnabled = true;

      # 120, 90, 60, 30, 12, 6, 2
      KeyRepeat = 90;

      # 120, 94, 68, 35, 25, 15
      InitialKeyRepeat = 15;

      "com.apple.mouse.tapBehavior" = 1;
      "com.apple.sound.beep.volume" = 0.0;
      "com.apple.sound.beep.feedback" = 0;
    };

    dock = {
      autohide = false;
      show-recents = false;
      launchanim = true;
      mouse-over-hilite-stack = true;
      orientation = "bottom";
      tilesize = 48;
    };

    finder = {
      AppleShowAllExtensions = true;
      ShowPathbar = true;
      FXDefaultSearchScope = "SCcf";
    };
    trackpad = {
      Clicking = true;
      TrackpadThreeFingerDrag = true;
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

