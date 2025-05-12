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
    atuin
    eza
    rclone
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
    poetry
    nodejs_22
    grip-grab
    rustup
    typst
  ];
}
