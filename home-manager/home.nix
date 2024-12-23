{ config, pkgs, inputs, ... }: {
  home.username = "hikary";
  home.homeDirectory = "/home/hikary";
  home.stateVersion = "24.11";

  nixpkgs.config.allowUnfree = true;

  nixpkgs.overlays = [
    (final: prev: {
      nixgl = import (builtins.fetchTarball {
        url = "https://github.com/nix-community/nixGL/archive/main.tar.gz";
      }) { };
    })
  ];

  home.sessionPath = [ "$HOME/.nix-profile/bin" ];

  xdg.mimeApps = {
    enable = true;
    defaultApplications = {
      "text/html" = [ "google-chrome.desktop" ];
      "x-scheme-handler/http" = [ "google-chrome.desktop" ];
      "x-scheme-handler/https" = [ "google-chrome.desktop" ];
      "x-scheme-handler/about" = [ "google-chrome.desktop" ];
      "x-scheme-handler/unknown" = [ "google-chrome.desktop" ];
    };
  };

  imports = [
    ./programs/kitty.nix
    ./programs/neovim
    ./programs/zsh
    # ./programs/tmux.nix
    ./programs/starship.nix
    ./programs/wut
    ./programs/hyprland
    ./programs/git
    ./programs/fastfetch
    ./programs/lazygit
    ./programs/easyeffects
    ./programs/opentabletdriver
    ./theme.nix
  ];

  home.sessionVariables = { NVIM_APPNAME = "nvim"; };

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
  };

  systemd.user.services = { 
    xdg-user-dirs-update.Install.WantedBy = [ ]; 
    auto-cpufreq = {
      Unit = {
        Description = "auto-cpufreq - Automatic CPU speed & power optimizer";
        After = ["network.target"];
      };

      Service = {
        Type = "simple";
        ExecStart = "${pkgs.auto-cpufreq}/bin/auto-cpufreq --daemon";
        Restart = "on-failure";
      };

      Install = {
        WantedBy = ["default.target"];
      };
    };
  };

  home.packages = with pkgs; [
    python312
    python312Packages.uv
    python312Packages.setuptools
    python3Packages.setuptools

    ripgrep
    fd
    tree-sitter
    curl
    nodejs_20
    jq
    shellcheck
    yamllint
    hadolint

    nixfmt-classic
    stylua
    shfmt
    ruff
    nodePackages.prettier

    google-chrome
    telegram-desktop
    vesktop
    dbeaver-bin
    easyeffects
    pulsemixer
    youtube-music
    nekoray
    blueman
    networkmanagerapplet

    xdg-user-dirs
    languagetool
    emptty
    opentabletdriver
    speedtest-rs
    libreoffice
    nemo
    osu-lazer-bin

    p7zip
    lrzip
    unrar
    unzip
    gnutar
    htop
    imv
    mpd
    noto-fonts-cjk-sans
    noto-fonts-emoji
    pamixer
    qbittorrent
    rsync
    xclip
    tmpmail
    slurp
    atuin
    httpie
    hurl
  ];

  programs.home-manager.enable = true;

  home.file.".profile".text = ''
    export SHELL="/home/hikary/.nix-profile/bin/zsh"
    [ -z "$ZSH_VERSION" ] && exec "$SHELL" -l
  '';

  xdg.systemDirs.data = [ "/usr/share" "/usr/local/share" ];
}
