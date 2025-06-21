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
  system.activationScripts = {
    postActivate.text = ''
      # Отключение автоматического переключения между рабочими столами (Spaces)
      # Предотвращает нежелательные переходы между Spaces при использовании Cmd+Tab или клике на иконки в Dock
      sudo -u ${user} defaults write com.apple.dock workspaces-auto-swoosh -bool NO
      # Отключение создания .DS_Store файлов на сетевых дисках
      # Предотвращает засорение сетевых папок служебными файлами macOS, что полезно при работе с Windows/Linux системами
      sudo -u ${user} defaults write com.apple.desktopservices DSDontWriteNetworkStores -bool TRUE
      # Отключение создания .DS_Store файлов на USB накопителях
      # Предотвращает создание скрытых служебных файлов на флешках и внешних дисках
      sudo -u ${user} defaults write com.apple.desktopservices DSDontWriteUSBStores -bool TRUE
      # Скрытие всех иконок на рабочем столе
      # Полностью убирает все файлы и папки с рабочего стола (файлы остаются, но становятся невидимыми)
      sudo -u ${user} defaults write com.apple.finder CreateDesktop TRUE # False для скрытия
      killall SystemUIServer
      killall Finder
      killall Dock
    '';
  };
  security.pam.services.sudo_local.reattach = true;
  security.pam.services.sudo_local.touchIdAuth = true;
  imports = [ ./modules ];
  system.stateVersion = 5;
}
