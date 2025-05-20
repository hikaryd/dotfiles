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
      "zed"
      "stats"
      "setapp"
      "upscayl"
      "flashspace"
      "loop"
      "ollama"
    ];
    brews = [ "bufbuild/buf/buf" ];
  };
}
