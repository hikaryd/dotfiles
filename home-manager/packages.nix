{ pkgs, lib, inputs, config, ... }: {
  nixpkgs.config.allowUnfree = true;
  nixpkgs.overlays = lib.mkForce [ inputs.nixgl.overlay ];

  home.packages = with pkgs; [
    inputs.nixgl.packages.${system}.nixGLDefault
    inputs.nixgl.packages.${system}.nixVulkanIntel
    zellij
    uwsm
    swww
    stu
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
    libnotify
    postgresql_16
    tabiew
    doh-proxy-rust
    repomix
    bluetuith
    uv

    p7zip
    lrzip
    unrar
    unzip
    gnutar

    pulsemixer

    xdg-desktop-portal
    xdg-user-dirs

    telegram-desktop
    ytmdesktop
    proxychains-ng
    ayugram-desktop
    sing-geosite

    # (config.lib.nixGL.wrap inputs.zen-browser.packages.${system}.default)
    (config.lib.nixGL.wrap beekeeper-studio)
    (config.lib.nixGL.wrap super-productivity)
    (config.lib.nixGL.wrap mpv)
    (config.lib.nixGL.wrap osu-lazer-bin)
    (config.lib.nixGL.wrap insomnia)
    (config.lib.nixGL.wrap google-chrome)

    nix-prefetch-scripts
    luajitPackages.luarocks

    docker
    docker-compose
    docker-buildx
    nekoray
    amdvlk
    bluez
    bluez-tools
    gnome-bluetooth
    networkmanagerapplet
    networkmanager
    wf-recorder
  ];
}
