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
      "notion"
      "font-caskaydia-cove-nerd-font"
      "Goooler/repo/hiddify"
      "drawio"
      "superwhisper"
      "flashspace"
      "rectangle-pro"
      # "firefox"
      "chatgpt"
      "claude-code"
      "dbeaver-community"
    ];
  };
}
