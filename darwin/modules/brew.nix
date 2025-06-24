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
    taps = [ "koekeishiya/formulae" ];
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
      "claude"
      "karabiner-elements"
      "cursor"
      "hammerspoon"
    ];
    brews = [ "spicetify-cli" "koekeishiya/formulae/yabai" ];
  };
}
