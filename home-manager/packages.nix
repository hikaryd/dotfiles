{ pkgs, lib, inputs, config, ... }: {
  nixpkgs.config.allowUnfree = true;
  nixpkgs.overlays =
    lib.mkForce [ inputs.nixgl.overlay inputs.hyprpanel.overlay ];

  home.packages = with pkgs; [
    inputs.nixgl.packages.${system}.nixGLDefault
    inputs.nixgl.packages.${system}.nixVulkanIntel
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
    uv
    libnotify
    postgresql_16
    tabiew
    doh-proxy-rust
    repomix

    python312Full
    python312Packages.pillow
    python312Packages.setproctitle
    python312Packages.toml

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

    (config.lib.nixGL.wrap inputs.zen-browser.packages.${system}.default)
    (config.lib.nixGL.wrap beekeeper-studio)
    (config.lib.nixGL.wrap super-productivity)
    (config.lib.nixGL.wrap zoom-us)
    (config.lib.nixGL.wrap mpv)

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

    # bar
    # dart-sass
    # gtk-layer-shell
    # gobject-introspection
  ];
}
