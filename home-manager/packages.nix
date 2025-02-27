{ pkgs, lib, inputs, config, ... }: {
  nixpkgs.config.allowUnfree = true;
  nixpkgs.overlays = lib.mkForce [ inputs.nixgl.overlay ];

  home.packages = with pkgs; [
    inputs.nixgl.packages.${system}.nixGLDefault
    inputs.nixgl.packages.${system}.nixVulkanIntel
    rustup
    todo
    ansible
    stu
    tree
    evince
    htop
    jan
    qbittorrent
    rsync
    less
    bluetuith
    proton-pass
    ncdu
    duf
    iotop
    powertop
    acpi
    ripgrep
    nemo
    jq
    fd
    imagemagick
    libnotify
    postgresql_16
    repomix
    uv
    grip-grab

    gajim

    flameshot
    satty

    p7zip
    lrzip
    unrar
    unzip
    gnutar

    pulsemixer
    tmuxinator

    xdg-desktop-portal

    telegram-desktop
    ayugram-desktop

    proxychains-ng
    sing-geosite

    pandoc
    typst

    tdf

    nmap

    (config.lib.nixGL.wrap keyguard)
    (config.lib.nixGL.wrap google-chrome)
    (config.lib.nixGL.wrap beekeeper-studio)
    (config.lib.nixGL.wrap mpv)
    (config.lib.nixGL.wrap osu-lazer-bin)
    (config.lib.nixGL.wrap insomnia)

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

    jamesdsp
  ];
}
