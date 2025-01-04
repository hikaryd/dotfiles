{ pkgs, inputs, ... }: {
  nixpkgs.config.allowUnfree = true;

  home.packages = with pkgs; [
    httpie
    tree
    htop
    rsync
    openssh_hpn
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

    p7zip
    lrzip
    unrar
    unzip
    gnutar

    pulsemixer

    xdg-desktop-portal
    xdg-user-dirs

    osu-lazer-bin
    telegram-desktop
    nekoray
    youtube-music
    dbeaver-bin
    python312
    (pkgs.writeShellScriptBin "zen" ''
      ${pkgs.nixgl.nixGLMesa}/bin/nixGLMesa ${
        inputs.zen-browser.packages.${system}.default
      }/bin/zen "$@"
    '')

    nix-prefetch-scripts
  ];
}
