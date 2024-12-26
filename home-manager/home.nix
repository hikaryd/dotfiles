{ pkgs, config, ... }: {
  home.username = "hikary";
  home.homeDirectory = "/home/hikary";
  home.stateVersion = "24.11";

  nixpkgs.config.allowUnfree = true;

  # Импортируем все конфиги
  imports = [
    ./programs/kitty.nix
    ./programs/neovim
    ./programs/zsh
    ./programs/starship.nix
    ./programs/hyprland
    ./programs/git
    ./programs/fastfetch
    ./programs/lazygit
    ./programs/easyeffects
    ./programs/opentabletdriver
    ./programs/windsurf.nix
    ./theme.nix
  ];

  home.sessionPath = [ "$HOME/.nix-profile/bin" ];

  home.packages = with pkgs; [
    # Базовые утилиты
    httpie
    tree
    htop
    rsync
    less
    ncdu
    duf
    yazi
    iotop
    powertop
    acpi
    ripgrep
    libreoffice
    nemo

    # Архиваторы
    p7zip
    lrzip
    unrar
    unzip
    gnutar

    # Сетевые утилиты
    speedtest-rs

    # Мультимедиа
    pulsemixer

    # XDG утилиты
    xdg-desktop-portal
    xdg-desktop-portal-gtk
    xdg-user-dirs

    # Шрифты
    noto-fonts-cjk-sans
    noto-fonts-emoji

    osu-lazer-bin
    telegram-desktop
  ];

  # Включаем home-manager
  programs.home-manager.enable = true;

  # Настройки окружения
  home.sessionVariables = {
    EDITOR = "nvim";
    VISUAL = "nvim";
    NVIM_APPNAME = "nvim";
  };

  # XDG настройки
  xdg = {
    enable = true;
    userDirs = {
      enable = true;
      createDirectories = true;
      desktop = "$HOME";
      documents = "$HOME/docs";
      download = "$HOME/Downloads";
      music = "$HOME/Downloads";
      pictures = "$HOME/Downloads";
      publicShare = "$HOME/Downloads";
      templates = "$HOME/Downloads";
      videos = "$HOME/Downloads";
    };
    systemDirs.data = [ "/usr/share" "/usr/local/share" ];
    mimeApps = {
      enable = true;
      defaultApplications = {
        "text/html" = [ "google-chrome.desktop" ];
        "x-scheme-handler/http" = [ "google-chrome.desktop" ];
        "x-scheme-handler/https" = [ "google-chrome.desktop" ];
        "x-scheme-handler/about" = [ "google-chrome.desktop" ];
        "x-scheme-handler/unknown" = [ "google-chrome.desktop" ];
      };
    };
  };

  # Системные сервисы
  systemd.user.services = {
    xdg-user-dirs-update.Install.WantedBy = [ ];
    auto-cpufreq = {
      Unit = {
        Description = "auto-cpufreq - Automatic CPU speed & power optimizer";
        After = [ "network.target" ];
      };
      Service = {
        Type = "simple";
        ExecStart = "${pkgs.auto-cpufreq}/bin/auto-cpufreq --daemon";
        Restart = "on-failure";
      };
      Install = { WantedBy = [ "default.target" ]; };
    };
    emptty = {
      Unit = {
        Description = "EmptTTY Display Manager";
        After = [ "network.target" ];
      };
      Service = {
        Type = "simple";
        ExecStart = "${pkgs.emptty}/bin/emptty";
        Restart = "always";
        RestartSec = "10";
        Environment = [
          "XDG_SESSION_TYPE=wayland"
          "XDG_CURRENT_DESKTOP=Hyprland"
          "PATH=${config.home.profileDirectory}/bin:/run/current-system/sw/bin:$PATH"
        ];
      };
      Install = { WantedBy = [ "default.target" ]; };
    };
  };

  home.file.".profile".text = ''
    export SHELL="/home/hikary/.nix-profile/bin/zsh"
    [ -z "$ZSH_VERSION" ] && exec "$SHELL" -l
  '';
}
