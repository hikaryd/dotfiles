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
      "raycast"
      "grishka/grishka/neardrop"
      "orion"
      "upscayl"
      "outline"
      "hyperkey"
      "chatgpt"
      "hammerspoon"
      "lihaoyun6/tap/quickrecorder"
      "betterdisplay"
      "caffeine"
      "pgadmin4"
      "lm-studio"
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
