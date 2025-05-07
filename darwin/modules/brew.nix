{ ... }: {
  homebrew = {
    brewPrefix = "/opt/homebrew/bin";
    enable = true;
    onActivation.cleanup = "zap";
    global = {
      brewfile = true;
      autoUpdate = true;
    };
    taps = [ "grishka/grishka" ];
    casks = [
      "telegram"
      "spotify"
      "1password-cli"
      "ghostty"
      "hiddenbar"
      "drawio"
      "dimentium/autoraise/autoraiseapp"
      "raycast"
      "loop"
      "grishka/grishka/neardrop"
    ];
    brews = [
      {
        name = "ollama";
        start_service = true;
      }
      "chainloop-cli"
    ];
  };
}
