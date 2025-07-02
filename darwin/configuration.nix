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
    shells = with pkgs; [ bash zsh ];
    systemPackages = with pkgs; [ coreutils neovim bat python3 tableplus ];
    systemPath = [
      "/usr/local/bin"
      "/opt/homebrew/bin"
      "/Users/${user}/.local/bin"
      "/Users/${user}/.cargo/bin"
    ];
    pathsToLink = [ "/Applications" ];
  };
  system = {
    primaryUser = "${user}";
    keyboard.enableKeyMapping = true;
    keyboard.swapLeftCtrlAndFn = true;
    startup.chime = true;
    defaults = {
      NSGlobalDomain = {
        # Dark mode
        AppleInterfaceStyle = "Dark";
        # Show all file extensions
        AppleShowAllExtensions = true;
        # Automatically hide and show the menu bar
        _HIHideMenuBar = true;
        # Disable window animations
        NSAutomaticWindowAnimationsEnabled = true;
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
        mru-spaces = false;
      };
      finder = {
        AppleShowAllExtensions = true;
        _FXShowPosixPathInTitle = true;
      };
      spaces.spans-displays = false;
    };
  };
  system.activationScripts.disableDSStoreCreation = {
    text = ''
      # Выполняем команды для отключения .DS_Store на сетевых и USB дисках
      /usr/bin/defaults write com.apple.desktopservices DSDontWriteNetworkStores -bool TRUE
      /usr/bin/defaults write com.apple.desktopservices DSDontWriteUSBStores -bool TRUE
    '';
  };

  security.pam.services.sudo_local.reattach = true;
  security.pam.services.sudo_local.touchIdAuth = true;
  imports = [ ./modules ];
  system.stateVersion = 5;
}
