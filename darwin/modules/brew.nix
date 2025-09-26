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
      "brave-browser"
      "font-victor-mono"
      "aldente"
      "rapidapi"
      "typora"
      "excalidrawz"
      "hazeover"
    ];
    brews =
      [ "codex" "neovim" "cliclick" "scrcpy" "koekeishiya/formulae/skhd" "gh" ];
  };
}
