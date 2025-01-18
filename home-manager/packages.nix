{ pkgs, lib, inputs, config, ... }: {
  nixpkgs.config.allowUnfree = true;
  nixpkgs.overlays =
    lib.mkForce [ inputs.nixgl.overlay inputs.hyprpanel.overlay ];

  home.packages = with pkgs; [
    inputs.nixgl.packages.${system}.nixGLDefault
    inputs.nixgl.packages.${system}.nixVulkanIntel
    zoom-us
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
    postgresql_16

    p7zip
    lrzip
    unrar
    unzip
    gnutar

    pulsemixer

    xdg-desktop-portal
    xdg-user-dirs

    telegram-desktop
    sing-geosite
    youtube-music
    dbeaver-bin

    (config.lib.nixGL.wrap inputs.zen-browser.packages.${system}.default)

    nekoray
    nix-prefetch-scripts
    luajitPackages.luarocks

    docker
    docker-compose
    docker-buildx
  ];
}
