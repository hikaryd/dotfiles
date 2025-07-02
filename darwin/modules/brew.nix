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
      "drawio"
      "raycast"
      "upscayl"
      "mos"
      "zen"
      "wakatime"
      "brave-browser"
      "karabiner-elements"
      "font-fira-code"
      "notion"
      "font-sf-pro"
      "ubar"
      "font-caskaydia-cove-nerd-font"
    ];
    taps = [ "FelixKratz/formulae" ];
    brews = [ "sketchybar" ];
  };
}
