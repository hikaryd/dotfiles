{ pkgs, system, ... }: {
  home.packages = with pkgs;
    if system == "x86_64-linux" then [ ghostty ] else [ ];

  xdg.configFile = {
    "ghostty/config" = {
      text = ''
        background-opacity = 0.8
        minimum-contrast = 1.1
        font-size = 12
        bold-is-bright = true

        theme = stylix
        background-blur-radius = 20
        background-blur = true
        macos-titlebar-style = tabs

        confirm-close-surface = false
        mouse-hide-while-typing = true

        window-padding-x = 10
        window-padding-y = 10
        window-padding-balance = true
        window-inherit-working-directory = true
        window-inherit-font-size = true

        scrollback-limit = 1000000
        custom-shader-animation = true
        keybind=ctrl+v=new_split:auto
        keybind=ctrl+h=goto_split:left
        keybind=ctrl+l=goto_split:right
        keybind=ctrl+k=goto_split:up
        keybind=ctrl+j=goto_split:down
        keybind=ctrl+shift+h=resize_split:left,20
        keybind=ctrl+shift+l=resize_split:right,20
        keybind=ctrl+shift+k=resize_split:up,20
        keybind=ctrl+shift+j=resize_split:down,20
      '';
    };

    "ghostty/themes/stylix" = {
      text = ''
        background = 11121d
        cursor-color = a0a8cd
        foreground = a0a8cd
        palette = 0=#11121d
        palette = 1=#ee6d85
        palette = 2=#95c561
        palette = 3=#d7a65f
        palette = 4=#7199ee
        palette = 5=#a485dd
        palette = 6=#9fbbf3
        palette = 7=#a0a8cd
        palette = 8=#353945
        palette = 9=#ee6d85
        palette = 10=#95c561
        palette = 11=#d7a65f
        palette = 12=#7199ee
        palette = 13=#a485dd
        palette = 14=#9fbbf3
        palette = 15=#bcc2dc
        selection-background = 212234
      '';
    };
  };
}
