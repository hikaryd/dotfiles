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
    taps = [ ];
    casks = [
      "telegram"
      "spotify"
      "1password-cli"
      "ghostty"
      "drawio"
      "raycast"
      "upscayl"
      "mos"
      "jordanbaird-ice"
      "zen"
      "wakatime"
      "brave-browser"
      "claude"
    ];
    brews = [ "spicetify-cli" ];
  };
}
