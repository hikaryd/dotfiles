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
      "drawio"
      "raycast"
      "grishka/grishka/neardrop"
      "orion"
      "upscayl"
      "outline"
      "chatgpt"
      "caffeine"
      "pgadmin4"
      "bartender"
      "zed"
      "lunar"
      "lm-studio"
    ];
    brews = [
      {
        name = "ollama";
        start_service = true;
      }
      "chainloop-cli"
      "bufbuild/buf/buf"
    ];
  };
}
