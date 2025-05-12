{ ... }: {
  services = {
    yabai = {
      enable = false;
      enableScriptingAddition = false;
      config = {
        layout = "bsp";
        window_gap = 12;
        top_padding = 20;
        bottom_padding = 20;
        left_padding = 20;
        right_padding = 20;
        window_placement = "second_child";
        focus_follows_mouse = "autofocus";
        # focus_follows_mouse = "off";
        mouse_follows_focus = "off";
        mouse_modifier = "alt";
        mouse_action1 = "move";
        mouser_action2 = "resize";
        mouse_drop_action = "swap";
      };
      extraConfig = ''
        yabai -m rule --add app="^System Settings$" manage=off
        yabai -m rule --add label="Finder" app="^Finder$" title="(Co(py|nnect)|Move|Info|Pref)" manage=off
        yabai -m rule --add label="Orion" app="^Orion$" title="^(General|(Tab|Password|Website|Extension)s|AutoFill|Se(arch|curity)|Privacy|Advance)$" manage=off
        yabai -m rule --add label="Arc" app="^Arc$" title="^(General|(Tab|Password|Website|Extension)s|AutoFill|Se(arch|curity)|Privacy|Advance)$" manage=off
        yabai -m rule --add label="System Preferences" app="^System Preferences$" title=".*" manage=off
        yabai -m rule --add label="App Store" app="^App Store$" manage=off
        yabai -m rule --add label="Activity Monitor" app="^Activity Monitor$" manage=off
        yabai -m rule --add label="KeePassXC" app="^KeePassXC$" manage=off
        yabai -m rule --add label="Calculator" app="^Calculator$" manage=off
        yabai -m rule --add label="Dictionary" app="^Dictionary$" manage=off
        yabai -m rule --add label="mpv" app="^mpv$" manage=off
        yabai -m rule --add label="Software Update" title="Software Update" manage=off
        yabai -m rule --add label="About This Mac" app="System Information" title="About This Mac" manage=off

        yabai -m signal --add app='^Ghostty$' event=window_created   action='yabai -m space --layout bsp'
        yabai -m signal --add app='^Ghostty$' event=window_destroyed action='yabai -m space --layout bsp'
      '';
    };

    skhd = {
      enable = false;
      skhdConfig = ''
        ################  ФОКУС ОКНА (hjkl)  ################
        alt - h : yabai -m window --focus west  || yabai -m display --focus west
        alt - j : yabai -m window --focus south || yabai -m display --focus south
        alt - k : yabai -m window --focus north || yabai -m display --focus north
        alt - l : yabai -m window --focus east  || yabai -m display --focus east

        ################  SWAP ОКОН  ################
        shift + alt - h : yabai -m window --swap west
        shift + alt - j : yabai -m window --swap south
        shift + alt - k : yabai -m window --swap north
        shift + alt - l : yabai -m window --swap east

        ################  RESIZE  (+/-20 px)  ################
        ctrl + shift - left : yabai -m window --resize left:-20:0
        ctrl + shift - right : yabai -m window --resize right:20:0
        ctrl + shift - up : yabai -m window --resize top:0:-20
        ctrl + shift - down : yabai -m window --resize bottom:0:20

        ################  TOGGLE FLOAT  ################
        alt - f : yabai -m window --toggle zoom-fullscreen

        ################  ЗАПУСК GHOSTTY  ################
        alt - return : open -na Ghostty

        ctrl + alt - h : yabai -m window --display west  && yabai -m display --focus west
        ctrl + alt - l : yabai -m window --display east  && yabai -m display --focus east

        ################  Фокус  ################
        cmd + shift - l : yabai -m display --focus next
        cmd + shift - h : yabai -m display --focus prev
      '';
    };
  };
}
