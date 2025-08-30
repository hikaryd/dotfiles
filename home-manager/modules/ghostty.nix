{ pkgs, ... }: {
  programs.ghostty = {
    enable = true;
    installBatSyntax = false;
    package = pkgs.emptyDirectory;
    settings = {
      font-family = "VictorMono";
      font-size = 15;
      theme = "light:tokyonight-day,dark:tokyonight";
      font-feature = "-liga +calt";
      font-thicken = true;
      window-colorspace = "display-p3";

      bold-is-bright = true;
      command = "/etc/profiles/per-user/tronin.egor/bin/nu";
      macos-titlebar-style = "tabs";

      background-blur-radius = 10;
      background-opacity = 0.7;
      background-blur = true;
      shell-integration-features = "no-cursor";
      macos-option-as-alt = true;
      keybind = [
        "ctrl+-=new_split:auto"
        "ctrl+h=goto_split:left"
        "ctrl+l=goto_split:right"
        "ctrl+k=goto_split:up"
        "ctrl+j=goto_split:down"
        "ctrl+shift+h=resize_split:left,20"
        "ctrl+shift+l=resize_split:right,20"
        "ctrl+shift+k=resize_split:up,20"
        "ctrl+shift+j=resize_split:down,20"
      ];
      selection-invert-fg-bg = true;

      confirm-close-surface = false;
      mouse-hide-while-typing = true;

      window-padding-x = 15;
      adjust-cell-height = "9%";
      window-padding-y = 10;
      mouse-scroll-multiplier = 2;
      term = "xterm-256color";

      cursor-style = "block";
      cursor-style-blink = true;
      cursor-opacity = 0.8;
      cursor-invert-fg-bg = true;

      window-save-state = "always";
      copy-on-select = true;
    };
  };
}
