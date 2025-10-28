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
    taps = [ "koekeishiya/formulae" "acsandmann/tap" ];
    casks = [
      "telegram"
      "telegram-desktop"
      "spotify"
      "1password-cli"
      "1password"
      "ghostty"
      "raycast"
      "mos"
      "wakatime"
      "karabiner-elements"
      "dbeaver-community"
      "topnotch"
      "aldente"
      "typora"
      "excalidrawz"
      "claude-code"
      "yaak"
      # fonts
      "font-symbols-only-nerd-font"
      "font-sf-mono"
      "font-sf-pro"
      "sf-symbols"
    ];
    brews = [ "codex" "neovim" "acsandmann/tap/rift" ];
  };
}
