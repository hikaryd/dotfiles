{ pkgs, ... }: {
  home.packages = with pkgs; [
    zsh
    eza
    zoxide
    rainfrog
    oxker
    csvlens
    repomix
    fzf
    file
    jq
    bat
    timg
    atuin
  ];

  home.file.".config/zsh" = {
    source = ./config;
    recursive = true;
  };

  home.file.".zshenv".text = ''
    export ZDOTDIR="$HOME/.config/zsh"
  '';

  programs = { zsh = { enable = true; }; };
}
