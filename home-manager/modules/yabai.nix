{ config, pkgs, ... }: {
  home.packages = with pkgs; [
    yabai
    skhd
  ];

  xdg.configFile."yabai/yabairc" = {
    executable = true;
    text = ''
      #!/usr/bin/env sh

      # общие настройки
      yabai -m config layout bsp
      yabai -m config window_placement second_child

      # отступы и промежутки (аналогично hyprland)
      yabai -m config top_padding    14
      yabai -m config bottom_padding 14
      yabai -m config left_padding   14
      yabai -m config right_padding  14
      yabai -m config window_gap     5

      # настройки мыши
      yabai -m config mouse_follows_focus on
      yabai -m config focus_follows_mouse autoraise
      yabai -m config mouse_modifier ctrl
      yabai -m config mouse_action1 move
      yabai -m config mouse_action2 resize
      yabai -m config mouse_drop_action swap

      # настройки разделения окон
      yabai -m config split_ratio 0.5
      yabai -m config auto_balance off

      # настройки внешнего вида
      yabai -m config window_shadow float
      yabai -m config window_opacity on
      yabai -m config active_window_opacity 1.0
      yabai -m config normal_window_opacity 0.7

      # настройка пространств (spaces), аналогично workspace в hyprland
      # мы будем использовать "label" для создания именованных пространств, 
      # как в hyprland
      
      # создаем пространства с метками
      yabai -m space 1 --label terminal
      yabai -m space 2 --label other
      yabai -m space 3 --label other2
      yabai -m space 4 --label misc
      yabai -m space 5 --label misc2
      yabai -m space 6 --label browser
      yabai -m space 8 --label telegram
      yabai -m space 10 --label music

      # правила для приложений, аналогично правилам в hyprland
      yabai -m rule --add app="^(zoom|zoom-us|Zoom Workplace|com.github.wwmm.easyeffects)$" space=misc
      yabai -m rule --add app="^(google-chrome|google-chrome-stable|com.ayugram.desktop|zen)$" space=browser opacity=0.9
      yabai -m rule --add app="^(mpv)$" manage=off
      yabai -m rule --add title="^(Google Chrome.*|Untitled - Google Chrome)$" manage=off
      yabai -m rule --add title="^(JamesDSP for Linux.*)$" manage=off
      yabai -m rule --add title="^(Untitled - Google Chrome.*)$" manage=off
      yabai -m rule --add app="^(org.telegram.desktop)$" space=telegram manage=off
      yabai -m rule --add app="^(pulsemixer|nekoray|hiddify|pavucontrol)$" manage=off
      yabai -m rule --add app="^(blueman-manager|nm-applet|nm-connection-editor)$" manage=off
      yabai -m rule --add app="^(org.kde.polkit-kde-authentication-agent-1)$" manage=off
      yabai -m rule --add app="^(bluetooth-manager|iwgtk)$" manage=off
      yabai -m rule --add app="^(1Password)$" manage=off grid=4:4:1:1:2:2
      yabai -m rule --add app="^(com.github.wwmm.easyeffects)$" manage=off grid=4:4:1:1:2:2
      yabai -m rule --add app="^(com-artemchep-keyguard-MainKt)$" manage=off grid=4:4:1:1:2:2
      yabai -m rule --add app="^(xdg-desktop-portal-gtk)$" manage=off grid=4:4:1:1:2:2
      yabai -m rule --add app="^(org.freedesktop.impl.portal.desktop.gtk|org.freedesktop.impl.portal.desktop.hyprland)$" opacity=0.8

      # исключения - приложения которые не будут управляться yabai
      yabai -m rule --add app="^System Settings$" manage=off
      yabai -m rule --add app="^Calculator$" manage=off
      yabai -m rule --add app="^Karabiner-Elements$" manage=off

      # запуск приложений при старте, аналогично exec-once в hyprland
      # так как yabai не имеет собственного функционала авто-запуска, 
      # используем задержку и открытие в фоне через open
      (sleep 2 && open -a "1Password") &
      (sleep 2 && open -a "zen-browser") &
      (sleep 2 && open -a "easyeffects") &
    '';
  };

  xdg.configFile."skhd/skhdrc" = {
    text = ''
      # Переключение между пространствами (аналоги hyprland workspace)
      ctrl - d : yabai -m space --focus terminal
      ctrl + shift - d : yabai -m window --space terminal --focus

      ctrl - 2 : yabai -m space --focus other
      ctrl + shift - 2 : yabai -m window --space other --focus

      ctrl - 3 : yabai -m space --focus other2
      ctrl + shift - 3 : yabai -m window --space other2 --focus

      ctrl - 4 : yabai -m space --focus misc
      ctrl + shift - 4 : yabai -m window --space misc --focus

      ctrl - 5 : yabai -m space --focus misc2
      ctrl + shift - 5 : yabai -m window --space misc2 --focus

      # Специальные пространства (аналоги special workspace в hyprland)
      # В Yabai нет прямого аналога специальных рабочих пространств,
      # поэтому используем обычные пространства и скрытие/показ
      ctrl - e : yabai -m space --focus telegram
      
      ctrl - v : yabai -m space --focus music
      
      ctrl - g : yabai -m space --focus browser
      ctrl + shift - g : yabai -m window --space browser --focus

      # Управление окнами
      ctrl - q : yabai -m window --close
      ctrl - f : yabai -m window --toggle float
      ctrl - space : yabai -m window --toggle zoom-fullscreen
      ctrl - period : yabai -m window --display next; yabai -m display --focus next
      ctrl - comma : yabai -m window --display prev; yabai -m display --focus prev

      # Перемещение окон
      ctrl + shift - h : yabai -m window --swap west
      ctrl + shift - j : yabai -m window --swap south
      ctrl + shift - k : yabai -m window --swap north
      ctrl + shift - l : yabai -m window --swap east

      # Перемещение фокуса
      ctrl - h : yabai -m window --focus west
      ctrl - j : yabai -m window --focus south
      ctrl - k : yabai -m window --focus north
      ctrl - l : yabai -m window --focus east

      # Запуск приложений
      ctrl - return : open -a "ghostty"
      ctrl + shift - s : screencapture -i -c
      ctrl - a : open -a "anyrun"

      # Дополнительные функции управления окнами
      ctrl + alt - r : yabai -m space --rotate 270
      ctrl + alt - x : yabai -m space --mirror x-axis
      ctrl + alt - y : yabai -m space --mirror y-axis
      ctrl + alt - e : yabai -m space --balance

      # Перемещение окон с созданием сплита (аналог warp в hyprland)
      ctrl + alt - h : yabai -m window --warp west
      ctrl + alt - j : yabai -m window --warp south
      ctrl + alt - k : yabai -m window --warp north
      ctrl + alt - l : yabai -m window --warp east

      # Управление стеками окон (аналог stack в hyprland)
      ctrl - s : yabai -m window --stack recent
      ctrl + shift - s : yabai -m window --stack next
      ctrl - p : yabai -m window --focus stack.prev
      ctrl - n : yabai -m window --focus stack.next

      # Управление сервисами
      ctrl + alt - q : yabai --stop-service
      ctrl + alt - s : yabai --start-service
      ctrl + alt - r : yabai --restart-service
      ctrl + alt - k : skhd --restart-service
    '';
  };
} 