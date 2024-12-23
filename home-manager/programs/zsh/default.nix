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
  ];

  home.file.".config/zsh" = {
    source = ./config;
    recursive = true;
  };

  home.file.".zshenv".text = ''
    export ZDOTDIR="$HOME/.config/zsh"
  '';

  programs = {
    zsh = {
      enable = true;
      shellAliases = {
        google-chrome = "google-chrome-wrapped";
        cat = "bat";
      };
    };

    bat = {
      enable = true;
      config = {
        theme = "Catppuccin-mocha";
        italic-text = "always";
        style = "numbers,changes,header";
        pager = "less -FR";
      };
      themes = {
        Catppuccin-mocha = builtins.readFile (pkgs.fetchurl {
          url = "https://raw.githubusercontent.com/catppuccin/bat/main/Catppuccin-mocha.tmTheme";
          hash = "sha256-qMQNJGZImmjrqzy7IiEkY5IhvPAMZpq0W6skLLsng/w=";
        });
      };
    };
  };
}
