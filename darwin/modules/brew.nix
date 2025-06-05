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
      "cursor"
      "upscayl"
      "mos"
      "slimhud"
      "mocki-toki/formulae/barik"
    ];
    brews = [ "bufbuild/buf/buf" ];
  };
}
