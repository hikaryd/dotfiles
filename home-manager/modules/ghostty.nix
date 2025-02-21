{ config, pkgs, ... }: {
  home.packages = with pkgs; [ (config.lib.nixGL.wrap ghostty) ];

  xdg.configFile."ghostty/tmux-session.sh" = {
    executable = true;
    text = ''
      #!/usr/bin/env bash
      export LANG="en_US.UTF-8"
      export LC_CTYPE="en_US.UTF-8"

      SESSION_NAME="ghostty"

      tmux list-sessions 2>/dev/null
      if [ $? -eq 1 ]; then
        exec tmux attach
      fi

      tmux has-session -t $SESSION_NAME 2>/dev/null
      if [ $? -eq 1 ]; then
        tmux new-session -d -s $SESSION_NAME
      fi

      exec tmux attach-session -t $SESSION_NAME
    '';
  };

  xdg.configFile."ghostty/config".text = ''
    # Настройки шрифта
    font-family = "Hack"
    font-size = 10
    font-thicken = true
    term = "xterm-256color"
    bold-is-bright = true
    adjust-box-thickness = 1

    # Запуск команды и интеграция с оболочкой
    command = ~/.config/ghostty/tmux-session.sh
    shell-integration = none

    # Тема и визуальное оформление
    # theme = "catppuccin-mocha"
    # theme = "iTerm2 Pastel Dark Background"
    theme = "cyberdream"
    background-opacity = 0.66
    background-blur-radius = 20

    cursor-style = block
    cursor-style-blink = true
    adjust-cursor-thickness = 1

    copy-on-select = true
    confirm-close-surface = false
    mouse-hide-while-typing = true

    window-theme = ghostty
    window-padding-x = 10
    window-padding-y = 10
    window-padding-balance = true
    window-padding-color = background
    window-inherit-working-directory = true
    window-inherit-font-size = true
    window-decoration = false

    gtk-titlebar = false
    gtk-single-instance = true
    gtk-wide-tabs = false

    scrollback-limit = 1000000
    custom-shader-animation = false
  '';

  xdg.configFile."ghostty/themes/cyberdream".text = ''
    palette = 0=#16181a
    palette = 1=#ff6e5e
    palette = 2=#5eff6c
    palette = 3=#f1ff5e
    palette = 4=#5ea1ff
    palette = 5=#bd5eff
    palette = 6=#5ef1ff
    palette = 7=#ffffff
    palette = 8=#3c4048
    palette = 9=#ff6e5e
    palette = 10=#5eff6c
    palette = 11=#f1ff5e
    palette = 12=#5ea1ff
    palette = 13=#bd5eff
    palette = 14=#5ef1ff
    palette = 15=#ffffff

    background = #16181a
    foreground = #ffffff
    cursor-color = #ffffff
    selection-background = #3c4048
    selection-foreground = #ffffff  
  '';
}
