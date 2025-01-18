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
    # Font
    font-family = "Maple Mono"
    font-size = 13
    font-thicken = true
    font-feature = ss01
    font-feature = ss04

    bold-is-bright = true
    adjust-box-thickness = 1
    command = ~/.config/ghostty/tmux-session.sh
    shell-integration = none

    # Theme
    theme = "catppuccin-mocha"
    # background-opacity = 0.66
    background-opacity = 0.75

    background-blur-radius = 40

    cursor-style = block
    cursor-style-blink = true
    adjust-cursor-thickness = 1

    copy-on-select = true
    confirm-close-surface = false
    mouse-hide-while-typing = true

    window-theme = ghostty
    window-padding-x = 4
    window-padding-y = 6
    window-padding-balance = true
    window-padding-color = background
    window-inherit-working-directory = true
    window-inherit-font-size = true
    window-decoration = false

    gtk-titlebar = false
    gtk-single-instance = true
    gtk-wide-tabs = false
  '';
}
