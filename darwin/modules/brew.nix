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
      "chatgpt"
      "claude-code"
      "dbeaver-community"
      "topnotch"
      "brave-browser"
      "font-victor-mono"
    ];
    brews = [
      "codex"
      # "koekeishiya/formulae/skhd"
      # "koekeishiya/formulae/yabai"
      "neovim"
      "cliclick"
    ];
  };
}
