{ pkgs, ... }: {
  nixpkgs.config.allowUnfree = true;

  home.packages = with pkgs; [
    htop
    tree
    less
    rsync
    p7zip
    asdf
    gping
    lrzip
    unrar
    unzip
    gnutar
    jq
    fd
    ripgrep
    luajitPackages.luarocks
    uv
    poetry
    nodejs_22
    grip-grab
    typst

    #apps
    upscayl
  ];
}
