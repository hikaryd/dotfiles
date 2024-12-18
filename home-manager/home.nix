{ pkgs, ... }: {
  home.username = "hikary";
  home.homeDirectory = "/home/hikary";
  home.stateVersion = "24.11";

  nixpkgs.config.allowUnfree = true;

  home.sessionPath = [ "$HOME/.nix-profile/bin" ];

  imports = [
    ./programs/kitty.nix
    ./programs/neovim
    ./programs/ags
    # ./programs/tmux.nix
    ./programs/starship.nix
    ./theme.nix
    ./programs/hyprland
  ];

  nixpkgs.overlays = [
    (final: prev: {
      nixgl = import (builtins.fetchTarball {
        url = "https://github.com/nix-community/nixGL/archive/main.tar.gz";
      }) { };
    })
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
    xdg-user-dirs-update = { Install.WantedBy = [ ]; };
  };

  home.packages = with pkgs; [
    python312
    python312Packages.uv
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
  ];

  programs.home-manager.enable = true;
}
