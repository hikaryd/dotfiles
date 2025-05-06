{ pkgs, system, ... }: {
  programs.ghostty = {
    enable = true;
    installBatSyntax = false;
    package =
      if system == "x86_64-linux" then pkgs.ghostty else pkgs.emptyDirectory;
    settings = {
      background-opacity = 0.8;
      minimum-contrast = 1.1;
      font-size = 12;
      bold-is-bright = true;

      # command = "${../../scripts/tmux-session.sh}";

      macos-titlebar-style = "tabs";
      background-blur-radius = 20;
      background-blur = true;
      keybind = [
        "ctrl+v=new_split:auto"
        "ctrl+h=goto_split:left"
        "ctrl+l=goto_split:right"
        "ctrl+k=goto_split:up"
        "ctrl+j=goto_split:down"
        "ctrl+shift+h=resize_split:left,20"
        "ctrl+shift+l=resize_split:right,20"
        "ctrl+shift+k=resize_split:up,20"
        "ctrl+shift+j=resize_split:down,20"
      ];

      confirm-close-surface = false;
      mouse-hide-while-typing = true;

      window-padding-x = 10;
      window-padding-y = 10;
      window-padding-balance = true;
      window-inherit-working-directory = true;
      window-inherit-font-size = true;

      scrollback-limit = 1000000;
      custom-shader-animation = true;
    };
  };
}
