{ pkgs, inputs, config, ... }:
let
  isDarwin = pkgs.stdenv.isDarwin;
  system = pkgs.system;
  nixglPkgs = inputs.nixgl.packages.${system};
  zenBrowserPkgs = inputs."zen-browser".packages.${system};
in {
  nixpkgs.config.allowUnfree = true;

  home.packages = let
    common = with pkgs; [
      htop
      tree
      less
      rsync
      p7zip
      lrzip
      unrar
      unzip
      gnutar
      jq
      fd
      ripgrep
      postgresql_16
      docker
      luajitPackages.luarocks
      uv
      nodejs_22
      grip-grab
      rustup
      typst
      telegram-desktop
    ];
    linuxOnly = if !isDarwin then
      with pkgs; [
        (config.lib.nixGL.wrap spotify)
        sing-geosite
        ansible
        bluetuith
        libnotify
        acpi
        ayugram-desktop
        repomix
        evince
        pulsemixer
        wf-recorder
        code-cursor
        qbittorrent
        duf
        powertop
        bluez
        bluez-tools
        networkmanager
        nemo
        blueman
        nixglPkgs.nixGLDefault
        nixglPkgs.nixVulkanIntel
        google-chrome
        slack
        insomnia
        zenBrowserPkgs.default
        (config.lib.nixGL.wrap mpv)
        (config.lib.nixGL.wrap osu-lazer-bin)
      ]
    else
      [ ];
    macosOnly = if isDarwin then with pkgs; [ chatgpt spotify ] else [ ];
  in common ++ linuxOnly ++ macosOnly;
}
