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
      sing-geosite
      grip-grab
      zenBrowserPkgs.default
    ];
    linuxOnly = if !isDarwin then
      with pkgs; [
        rustup
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
        typst
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
        (config.lib.nixGL.wrap mpv)
        (config.lib.nixGL.wrap osu-lazer-bin)
      ]
    else
      [ ];
    macosOnly = if isDarwin then with pkgs; [ mas chatgpt ] else [ ];
  in common ++ linuxOnly ++ macosOnly;
}
