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
      "apple-juice"
      "claude"
      "ollama"
      "orion"
      "jordanbaird-ice"
    ];
    brews = [ "bufbuild/buf/buf" "zoxide" ];
  };
}
