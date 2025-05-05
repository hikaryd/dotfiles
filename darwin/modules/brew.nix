{ ... }: {
  homebrew = {
    brewPrefix = "/opt/homebrew/bin";
    enable = true;
    caskArgs.no_quarantine = true;
    onActivation.cleanup = "zap";
    global = {
      brewfile = true;
      autoUpdate = true;
    };
    taps = [ ];
    casks = [
      "lm-studio"
      "raycast"
      "telegram"
      "spotify"
      "zappy"
      "1password-cli"
      "ghostty"
      "hiddenbar"
      "apple-juice"
    ];
    brews = [
      "libiconv"
      "switchaudio-osx"
      "nowplaying-cli"
      "fonttools"
      {
        name = "ollama";
        start_service = true;
      }
    ];
  };
}
