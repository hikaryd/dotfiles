{ pkgs, inputs, config, ... }: {
  nixpkgs.config.allowUnfree = true;

  home.packages = with pkgs; [
    # Графические драйверы и совместимость
    inputs.nixgl.packages.${system}.nixGLDefault
    inputs.nixgl.packages.${system}.nixVulkanIntel

    # Системные утилиты и мониторинг
    htop
    duf
    powertop
    acpi
    tree
    less
    bluetuith
    libnotify
    xdg-desktop-portal
    bluez
    bluez-tools
    networkmanager
    libsecret
    pass-wayland

    # Файловые менеджеры и работа с файлами
    nemo
    rsync
    p7zip
    lrzip
    unrar
    unzip
    gnutar

    # Разработка и программирование
    rustup
    vscode
    jq
    fd
    ripgrep
    postgresql_16
    docker
    docker-compose
    docker-buildx
    luajitPackages.luarocks
    nix-prefetch-scripts
    ansible
    uv
    nodejs_22
    code-cursor

    # Инструменты для работы с текстом и документами
    pandoc
    typst
    tdf
    python313Packages.weasyprint

    # Интернет и сеть
    (config.lib.nixGL.wrap google-chrome)
    (config.lib.nixGL.wrap slack)
    qbittorrent
    proxychains-ng
    sing-geosite
    nekoray
    ayugram-desktop
    (config.lib.nixGL.wrap insomnia)
    grip-grab
    tidal-hifi

    # Офисные приложения и просмотр документов
    onlyoffice-desktopeditors
    evince

    # Мультимедиа и развлечения
    (config.lib.nixGL.wrap mpv)
    (config.lib.nixGL.wrap osu-lazer-bin)
    wf-recorder
    imagemagick

    # Инструменты командной строки и улучшения терминала 
    tmuxinator
    pulsemixer
    repomix
  ];
}
