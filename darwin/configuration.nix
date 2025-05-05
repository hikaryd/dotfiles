{ pkgs, user, ... }: {
  nixpkgs = {
    config.allowUnfree = true;
    config.allowUnfreePredicate = (_: true);
  };

  nix.enable = false;
  # nix.settings = { experimental-features = "nix-command flakes"; };
  # nix.optimise.automatic = true;

  users.users."${user}" = {
    name = "${user}";
    home = "/Users/${user}";
    shell = pkgs.nushell;
  };

  environment = {
    shells = with pkgs; [ bash zsh nushell ];
    systemPackages = [ pkgs.coreutils ];
    systemPath = [
      "/usr/local/bin"
      "/opt/homebrew/bin"
      "/Users/${user}/.local/bin"
      "/Users/${user}/.cargo/bin"
      "/Users/${user}/Library/pnpm"
      "/Users/${user}/.bun/bin"
      "/Users/${user}/.lmstudio/bin"
    ];
    pathsToLink = [ "/Applications" ];
  };

  system = {
    keyboard.enableKeyMapping = true;
    keyboard.remapCapsLockToEscape = true;

    startup.chime = true;

    activationScripts.postUserActivation.text = ''
      # disable .DS_Store files
      defaults write com.apple.desktopservices DSDontWriteNetworkStores -bool TRUE
      defaults write com.apple.desktopservices DSDontWriteUSBStores -bool TRUE

      # hide icons on desktop
      defaults write com.apple.finder CreateDesktop FALSE; killall Finder
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
        # Automatically hide and show the Dock
        autohide = true;
        autohide-delay = 0.0;
        autohide-time-modifier = 0.45;

        # Style options
        orientation = "left";
        show-recents = false;

        persistent-apps = [ "/Applications/Ghostty.app/" ];
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
