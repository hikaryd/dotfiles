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
      shell-integration = "detect";
      shell-integration-features = "no-cursor";
      macos-option-as-alt = true;
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

        # Ctrl+Shift+hjkl => CSI u with Ctrl+Shift modifier (6)
        "ctrl+shift+h=csi:104;6u"
        "ctrl+shift+j=csi:106;6u"
        "ctrl+shift+k=csi:107;6u"
        "ctrl+shift+l=csi:108;6u"

        # Examples (disabled) for splits navigation/resizing
        # "ctrl+-=new_split:auto"
        # "ctrl+h=goto_split:left"
        # "ctrl+l=goto_split:right"
        # "ctrl+k=goto_split:up"
        # "ctrl+j=goto_split:down"
        # "ctrl+shift+h=resize_split:left,20"
        # "ctrl+shift+l=resize_split:right,20"
        # "ctrl+shift+k=resize_split:up,20"
        # "ctrl+shift+j=resize_split:down,20"
      ];
      selection-invert-fg-bg = true;

      confirm-close-surface = false;
      mouse-hide-while-typing = true;

      window-padding-x = 15;
      adjust-cell-height = "9%";
      window-padding-y = 10;
      mouse-scroll-multiplier = 2;
      term = "xterm-256color";
      working-directory = "inherit";
      window-inherit-working-directory = true;

      cursor-style = "block";
      cursor-style-blink = true;
      cursor-opacity = 0.8;
      cursor-invert-fg-bg = true;

      window-save-state = "always";
      copy-on-select = true;
    };
  };
}
