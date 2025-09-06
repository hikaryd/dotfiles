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
      "telegram-desktop"
      "spotify"
      "1password-cli"
      "1password"
      "ghostty"
      "raycast"
      "mos"
      "wakatime"
      "karabiner-elements"
      # "notion"
      "font-caskaydia-cove-nerd-font"
      "drawio"
      # "superwhisper"
      # "flashspace"
      # "rectangle-pro"
      "firefox"
      "claude-code"
      "dbeaver-community"
      "topnotch"
      "brave-browser"
      "font-victor-mono"
      "aldente"
      "jordanbaird-ice"
    ];
    brews = [
      "codex"
      # "koekeishiya/formulae/skhd"
      # "koekeishiya/formulae/yabai"
      "neovim"
      "cliclick"
      "scrcpy"
    ];
  };
}
