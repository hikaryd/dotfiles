{ ... }: {
  homebrew = {
    brewPrefix = "/opt/homebrew/bin";
    enable = true;
    onActivation.cleanup = "zap";
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
      "outline"
      "cursor"
      "stats"
      "upscayl"
      "flashspace"
      "loop"
      "ollama"
      "iina"
    ];
    brews = [ "bufbuild/buf/buf" ];
  };
}
