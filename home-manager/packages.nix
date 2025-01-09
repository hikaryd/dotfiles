{ pkgs, inputs, ... }: {
  nixpkgs.config.allowUnfree = true;

  home.packages = with pkgs; [
    tree
    htop
    rsync
    less
    ncdu
    duf
    iotop
    powertop
    acpi
    ripgrep
    libreoffice
    nemo
    fd
    imagemagick
    uv
    libnotify

    p7zip
    lrzip
    unrar
    unzip
    gnutar

    pulsemixer

    xdg-desktop-portal
    xdg-user-dirs

    telegram-desktop
    nekoray
    sing-geosite
    youtube-music
    dbeaver-bin

    (pkgs.writeShellScriptBin "zen" ''
      ${pkgs.nixgl.nixGLMesa}/bin/nixGLMesa ${
        inputs.zen-browser.packages.${system}.default
      }/bin/zen "$@"
    '')

    nix-prefetch-scripts

    docker
    docker-compose
    docker-buildx
  ];
}
