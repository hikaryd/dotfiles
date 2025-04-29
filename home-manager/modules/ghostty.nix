{ pkgs, system, ... }: {
  home.packages = with pkgs;
    if system == "x86_64-linux" then [ kanata ] else [ ];

  xdg.configFile = {
    "ghostty/config" = {
      text = ''
        background-opacity = 0.8
        minimum-contrast = 1.1
        font-size = 10
        term = xterm-256color
        bold-is-bright = true

        command = ${../../scripts/tmux-session.sh}
        linux-cgroup = single-instance

        theme = stylix
        background-blur-radius = 20

        confirm-close-surface = false
        mouse-hide-while-typing = true

        window-padding-x = 10
        window-padding-y = 10
        window-padding-balance = true
        window-inherit-working-directory = true
        window-inherit-font-size = true

        scrollback-limit = 1000000
        custom-shader-animation = true
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
