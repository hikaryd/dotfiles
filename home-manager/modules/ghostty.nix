{ pkgs, ... }: {
  programs.ghostty = {
    enable = true;
    installBatSyntax = false;
    package = pkgs.emptyDirectory;
    settings = {
      theme = "light:Rose Pine Dawn,dark:Rose Pine";
      # theme = "light:tokyonight-day,dark:tokyonight";

      # font-family = "VictorMono";
      adjust-cell-height = "45%";
      font-family = "Berkeley Mono";
      font-size = 15;

      background-opacity = 0.95;
      background-blur = true;
      # font-feature = "-liga +calt";
      # font-thicken = true;
      # window-colorspace = "display-p3";
      # bold-is-bright = true;

      confirm-close-surface = false;

      cursor-style-blink = false;

      macos-option-as-alt = false;
      macos-titlebar-style = "hidden";
      window-padding-balance = true;

      cursor-style = "block";
      mouse-hide-while-typing = true;
      # cursor-opacity = 0.8;
      # cursor-invert-fg-bg = true;
      # cursor-style-blink = true;
      shell-integration-features = "no-cursor";
      cursor-color = "cell-foreground";
      cursor-text = "cell-background";

      # selection-invert-fg-bg = true;
      title = " ";

      # window-padding-x = 15;
      # window-padding-y = 10;
      window-padding-x = 6;
      window-padding-y = 6;

      command = "/etc/profiles/per-user/tronin.egor/bin/nu";
      keybind = [
        "ctrl+1=csi:49;5u"
        "ctrl+2=csi:50;5u"
        "ctrl+3=csi:51;5u"
        "ctrl+4=csi:52;5u"
        "ctrl+5=csi:53;5u"
        "ctrl+6=csi:54;5u"
        "ctrl+7=csi:55;5u"
        "ctrl+8=csi:56;5u"
        "ctrl+9=csi:57;5u"
      ];

      # term = "xterm-256color";
      # working-directory = "inherit";
      # window-inherit-working-directory = true;
      # window-save-state = "always";
      # copy-on-select = true;
    };
  };
}
