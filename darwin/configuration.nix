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
    systemPackages = with pkgs; [ coreutils neovim bat python3 rustup ];
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

    startup.chime = false;

    defaults = {
      NSGlobalDomain = {
        # Тёмная тема
        AppleInterfaceStyle = "Dark";
        # Всегда показывать расширения файлов
        AppleShowAllExtensions = true;
        # Скрывать меню-бар
        _HIHideMenuBar = true;
        # Отключать анимации окон
        NSAutomaticWindowAnimationsEnabled = false;
        # Скорость трекпада
        "com.apple.trackpad.scaling" = 0.7;

        AppleMeasurementUnits = "Centimeters";
        AppleMetricUnits = 1;
        AppleTemperatureUnit = "Celsius";

        # Поведение клавиш повтора
        InitialKeyRepeat = 15;
        KeyRepeat = 2;

        # Полосы прокрутки: показывать при скролле
        AppleShowScrollBars = "WhenScrolling";

        # Расширенный диалог сохранения
        NSNavPanelExpandedStateForSaveMode = true;
        NSNavPanelExpandedStateForSaveMode2 = true;

        # Полный доступ клавиатурой: Tab переключает все элементы
        AppleKeyboardUIMode = 3;

        # Текстовые «умные» фичи
        NSAutomaticQuoteSubstitutionEnabled = false; # умные кавычки
        NSAutomaticSpellingCorrectionEnabled = false; # автокоррекция
        NSAutomaticCapitalizationEnabled = false; # авто-заглавные
        NSAutomaticDashSubstitutionEnabled = false; # -- → —
        NSAutomaticPeriodSubstitutionEnabled = false; # два пробела → точка
      };

      # Finder 
      finder = {
        AppleShowAllExtensions = true; # все расширения
        AppleShowAllFiles = true; # показывать скрытые файлы
        _FXSortFoldersFirst = true; # папки выше файлов
        ShowPathbar = true; # панель пути внизу
        ShowStatusBar = false; # статус-бар скрыт
        QuitMenuItem = true; # можно выйти из Finder через ⌘Q
        _FXShowPosixPathInTitle = true; # полный POSIX-путь в заголовке
        FXEnableExtensionChangeWarning =
          false; # не спрашивать при смене расширения
        FXDefaultSearchScope = "SCcf"; # поиск по умолчанию: текущая папка
      };

      # Dock 
      dock = {
        autohide = true; # автоскрытие Dock
        autohide-delay = 0.0; # показывать мгновенно
        autohide-time-modifier = 0.45; # скорость анимации
        orientation = "right"; # справа
        show-recents = false; # без «Недавних»
        persistent-apps = [ ]; # начисто
        mru-spaces = false; # не пересортировывать рабочие столы

        tilesize = 20; # размер иконок
        show-process-indicators = true; # точки у запущенных приложений
      };

      spaces.spans-displays = false;
      screencapture = {
        location = "/Users/${user}/Desktop/screenshots";
        type = "png";
      };
    };
  };

  # Показать ~/Library
  system.activationScripts.unhideUserLibrary.text = ''
    /usr/bin/chflags nohidden "/Users/${user}/Library" 2>/dev/null || true
  '';

  # Перезапустить Finder и Dock, чтобы настройки применились сразу после switch
  system.activationScripts.restartUiApps.text = ''
    /usr/bin/killall Finder 2>/dev/null || true
    /usr/bin/killall Dock 2>/dev/null || true
  '';

  security.pam.services.sudo_local.reattach = true;
  security.pam.services.sudo_local.touchIdAuth = true;

  imports = [ ./modules ];
  system.stateVersion = 5;
}
