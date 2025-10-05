{ ... }: {
  homebrew = {
    brewPrefix = "/opt/homebrew/bin";
    enable = true;
    onActivation = {
      autoUpdate = true;
      upgrade = true;
      cleanup = "uninstall";
    };
    global = {
      brewfile = true;
      autoUpdate = true;
    };
    taps = [ "koekeishiya/formulae" ];
    casks = [
      "telegram"
      "telegram-desktop"
      "spotify"
      "1password-cli"
      "1password"
      "ghostty"
      "raycast"
      "mos"
      "wakatime"
      "karabiner-elements"
      "dbeaver-community"
      "topnotch"
      "brave-browser"
      "nikitabobko/tap/aerospace"
      "aldente"
      "rapidapi"
      "typora"
      "excalidrawz"
      "claude-code"
      # fonts
      "font-symbols-only-nerd-font"
      "font-sf-mono"
      "font-sf-pro"
      "sf-symbols"
    ];
    brews = [ "codex" "neovim" "koekeishiya/formulae/skhd" ];
  };
}
