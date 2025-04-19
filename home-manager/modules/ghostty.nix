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
    font-family = "Fira Code"
    # font-family = "Liga SFMono Nerd Font"
    font-size = 10
    font-thicken = false
    adjust-box-thickness = 0
    term = "xterm-256color"
    bold-is-bright = true

    # Запуск команды и интеграция с оболочкой
    command = ~/.config/ghostty/tmux-session.sh
    shell-integration = detect
    shell-integration-features = cursor,sudo,title
    linux-cgroup = single-instance

    # Тема и визуальное оформление
    theme = "catppuccin-mocha"
    # theme = "iTerm2 Pastel Dark Background"
    background-opacity = 0.8
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
    gtk-single-instance = false
    gtk-wide-tabs = false

    scrollback-limit = 1000000
    custom-shader-animation = true
  '';
}
