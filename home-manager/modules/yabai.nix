{ pkgs, ... }: {
  home.packages = with pkgs; [ yabai skhd ];

  services.yabai = {
    enable = true;
    package = pkgs.yabai;
    enableScriptingAddition = true;
  };

  services.skhd = {
    enable = true;
    package = pkgs.skhd;
  };

  xdg.configFile."yabai/yabairc" = {
    executable = true;
    text = ''
      #!/usr/bin/env sh

      # загрузка скрипта управления пространствами
      yabai -m config layout bsp
      yabai -m config window_placement second_child

      # отступы и промежутки
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

      # настройки окон
      yabai -m config split_ratio 0.5
      yabai -m config auto_balance off

      # внешний вид окон
      yabai -m config window_shadow float
      yabai -m config window_opacity on
      yabai -m config active_window_opacity 1.0
      yabai -m config normal_window_opacity 0.7

      # Настройка пространств
      yabai -m space 1 --label terminal
      yabai -m space 2 --label web
      yabai -m space 3 --label code
      yabai -m space 4 --label chat
      yabai -m space 5 --label files
      yabai -m space 6 --label media

      # Правила для окон
      yabai -m rule --add app="^(Google Chrome|Firefox|Safari)$" space=web
      yabai -m rule --add app="^(Visual Studio Code|PyCharm|IntelliJ IDEA)$" space=code
      yabai -m rule --add app="^(Telegram|Discord|Slack)$" space=chat
      yabai -m rule --add app="^(Finder|File Manager)$" space=files
      yabai -m rule --add app="^(mpv|VLC|Spotify)$" space=media

      # Исключения - приложения, которыми не будет управлять yabai
      yabai -m rule --add app="^System Settings$" manage=off
      yabai -m rule --add app="^Calculator$" manage=off
      yabai -m rule --add app="^1Password$" manage=off grid=4:4:1:1:2:2

      # Автозапуск приложений при старте yabai
      # Проверяем, запущены ли уже программы, чтобы избежать дублирования

      # Terminal
      if ! pgrep -f "kanata" > /dev/null; then
        open -a "kanata"
      fi
    '';
  };

  xdg.configFile."skhd/skhdrc" = {
    text = ''
      # Переключение между пространствами
      ctrl - 1 : yabai -m space --focus terminal
      ctrl - 2 : yabai -m space --focus web
      ctrl - 3 : yabai -m space --focus code
      ctrl - 4 : yabai -m space --focus chat
      ctrl - 5 : yabai -m space --focus files
      ctrl - 6 : yabai -m space --focus media

      # Отправка окна в пространство
      ctrl + shift - 1 : yabai -m window --space terminal --focus
      ctrl + shift - 2 : yabai -m window --space web --focus
      ctrl + shift - 3 : yabai -m window --space code --focus
      ctrl + shift - 4 : yabai -m window --space chat --focus
      ctrl + shift - 5 : yabai -m window --space files --focus
      ctrl + shift - 6 : yabai -m window --space media --focus

      # Управление окнами
      ctrl - q : yabai -m window --close
      ctrl - f : yabai -m window --toggle float
      ctrl - space : yabai -m window --toggle zoom-fullscreen

      # Переключение дисплеев
      ctrl - period : yabai -m display --focus next
      ctrl - comma : yabai -m display --focus prev

      # Перемещение на другой дисплей
      ctrl + shift - period : yabai -m window --display next; yabai -m display --focus next
      ctrl + shift - comma : yabai -m window --display prev; yabai -m display --focus prev

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

      # Запуск терминала
      ctrl - return : open -a "ghostty"

      # Скриншот
      ctrl + shift - s : screencapture -i -c

      # Дополнительные функции
      ctrl + alt - r : yabai -m space --rotate 270
      ctrl + alt - x : yabai -m space --mirror x-axis
      ctrl + alt - y : yabai -m space --mirror y-axis
      ctrl + alt - e : yabai -m space --balance

      # Управление сервисами
      ctrl + alt - q : yabai --stop-service
      ctrl + alt - s : yabai --start-service
      ctrl + alt - r : yabai --restart-service
      ctrl + alt - k : skhd --restart-service
    '';
  };

}

