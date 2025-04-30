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
  launchd.daemons = {
    # karabiner-virtualhiddevice = {
    #   script = ''
    #     '/Library/Application Support/org.pqrs/Karabiner-DriverKit-VirtualHIDDevice/Applications/Karabiner-VirtualHIDDevice-Daemon.app/Contents/MacOS/Karabiner-VirtualHIDDevice-Daemon'
    #   '';
    #   serviceConfig = {
    #     Label = "org.custom.karabiner-virtualhiddevice";
    #     RunAtLoad = true;
    #     KeepAlive = true;
    #     StandardOutPath = "/var/log/karabiner.out.log";
    #     StandardErrorPath = "/var/log/karabiner.err.log";
    #   };
    # };
    #
    # kanata = {
    #   script = ''
    #     /etc/profiles/per-user/tronin.egor/bin/kanata -c '/Users/${user}/.config/kanata/kanata.kbd'
    #   '';
    #   serviceConfig = {
    #     Label = "org.custom.kanata";
    #     RunAtLoad = true;
    #     KeepAlive = true;
    #     StandardOutPath = "/var/log/kanata.out.log";
    #     StandardErrorPath = "/var/log/kanata.err.log";
    #   };
    # };
    #
    # "1password" = {
    #   script = ''
    #     /Applications/1Password.app/Contents/MacOS/1Password --silent
    #   '';
    #   serviceConfig = {
    #     Label = "org.custom.1password";
    #     RunAtLoad = true;
    #     KeepAlive = true;
    #     StandardOutPath = "/var/log/1password.out.log";
    #     StandardErrorPath = "/var/log/1password.err.log";
    #   };
    # };
  };

  homebrew = {
    enable = true;
    casks = [ "ghostty" ];

  };

  # services.yabai = {
  #   enable = true;
  #   package = pkgs.yabai;
  #   enableScriptingAddition = true;
  # };
  #
  # services.skhd = {
  #   enable = true;
  #   package = pkgs.skhd;
  # };
}

