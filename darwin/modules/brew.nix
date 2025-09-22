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
      "font-caskaydia-cove-nerd-font"
      "dbeaver-community"
      "topnotch"
      # "brave-browser"
      "font-victor-mono"
      "aldente"
      "rapidapi"
      # "nikitabobko/tap/aerospace"
      "typora"
      "excalidrawz"
      "hazeover"
      "koekeishiya/formulae/skhd"
    ];
    brews = [ "codex" "neovim" "cliclick" "scrcpy" ];
  };
}
