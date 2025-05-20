{ pkgs, user, ... }: {
  nixpkgs = {
    config.allowUnfree = true;
    config.allowUnfreePredicate = (_: true);
  };

  nix.enable = false;

  users.users."${user}" = {
    name = "${user}";
    home = "/Users/${user}";
  };

  environment = {
    shells = with pkgs; [ bash zsh nushell ];
    systemPackages = [
      pkgs.coreutils
      pkgs.neovim
      pkgs.eza
      pkgs.bat
      pkgs.yazi
      pkgs.atuin
      pkgs.python3
    ];
    systemPath = [
      "/usr/local/bin"
      "/opt/homebrew/bin"
      "/Users/${user}/.local/bin"
      "/Users/${user}/.cargo/bin"
    ];
    pathsToLink = [ "/Applications" ];
  };

  programs.direnv = {
    enable = true;
    direnvrcExtra = ''
      echo "Loaded direnv "
    '';
    silent = true;
  };

  system = {
    keyboard.enableKeyMapping = true;
    keyboard.swapLeftCtrlAndFn = true;

    startup.chime = true;

    activationScripts.postUserActivation.text = ''
      # disable .DS_Store files
      defaults write com.apple.desktopservices DSDontWriteNetworkStores -bool TRUE
      defaults write com.apple.desktopservices DSDontWriteUSBStores -bool TRUE

      # hide icons on desktop
      defaults write com.apple.finder CreateDesktop FALSE; killall Finder
      defaults write com.apple.spaces spans-displays -bool true && killall SystemUIServer
    '';

    defaults = {
      NSGlobalDomain = {
        # Dark mode
        AppleInterfaceStyle = "Dark";
        # Show all file extensions
        AppleShowAllExtensions = true;
        # Automatically hide and show the menu bar
        _HIHideMenuBar = false;
        # Disable window animations
        NSAutomaticWindowAnimationsEnabled = false;

        "com.apple.trackpad.scaling" = 0.7;
        AppleMeasurementUnits = "Centimeters";
        AppleMetricUnits = 1;
        AppleShowScrollBars = "Automatic";
        AppleTemperatureUnit = "Celsius";
        InitialKeyRepeat = 15;
        KeyRepeat = 2;
      };

      screencapture = {
        location = "/Users/${user}/Desktop/screenshots";
        type = "png";
      };
      dock = {
        autohide = true;
        autohide-delay = 0.0;
        autohide-time-modifier = 0.45;

        # Style options
        orientation = "left";
        show-recents = false;

        persistent-apps = [ ];
      };
      finder = {
        AppleShowAllExtensions = true;
        _FXShowPosixPathInTitle = true;
      };
    };
  };

  security.pam.services.sudo_local.touchIdAuth = true;

  imports = [ ./modules ];

  system.stateVersion = 5;
}
