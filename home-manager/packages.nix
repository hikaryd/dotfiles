{ pkgs, lib, inputs, config, ... }: {
  nixpkgs.config.allowUnfree = true;
  nixpkgs.overlays = lib.mkForce [ inputs.nixgl.overlay ];

  home.packages = with pkgs; [
    inputs.nixgl.packages.${system}.nixGLDefault
    inputs.nixgl.packages.${system}.nixVulkanIntel
    rustup
    swww
    stu
    tree
    htop
    jan
    rsync
    less
    proton-pass
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
    grip-grab

    p7zip
    lrzip
    unrar
    unzip
    gnutar

    pulsemixer
    tmuxinator

    xdg-desktop-portal
    xdg-user-dirs

    telegram-desktop
    ytmdesktop
    proxychains-ng
    ayugram-desktop
    sing-geosite

    (config.lib.nixGL.wrap beekeeper-studio)
    (config.lib.nixGL.wrap super-productivity)
    (config.lib.nixGL.wrap mpv)
    (config.lib.nixGL.wrap osu-lazer-bin)
    (config.lib.nixGL.wrap insomnia)
    (config.lib.nixGL.wrap google-chrome)

    nix-prefetch-scripts
    luajitPackages.luarocks
    gitu

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
