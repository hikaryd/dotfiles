{ pkgs, ... }: {
  # home.packages = with pkgs; [ yabai skhd ];

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
      yabai -m config mouse_modifier alt
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

      # Запуск kanata для переназначения клавиш (не для управления yabai)
      if ! pgrep -f "kanata" > /dev/null; then
        ${pkgs.kanata}/bin/kanata --cfg ~/.config/kanata/kanata.kbd &
      fi
    '';
  };

  xdg.configFile."skhd/skhdrc" = {
    text = ''
      # Переключение между пространствами
      cmd - 1 : yabai -m space --focus terminal
      cmd - 2 : yabai -m space --focus web
      cmd - 3 : yabai -m space --focus code
      cmd - 4 : yabai -m space --focus chat
      cmd - 5 : yabai -m space --focus files
      cmd - 6 : yabai -m space --focus media

      # Отправка окна в пространство
      cmd + shift - 1 : yabai -m window --space terminal --focus
      cmd + shift - 2 : yabai -m window --space web --focus
      cmd + shift - 3 : yabai -m window --space code --focus
      cmd + shift - 4 : yabai -m window --space chat --focus
      cmd + shift - 5 : yabai -m window --space files --focus
      cmd + shift - 6 : yabai -m window --space media --focus

      # Управление окнами
      cmd - q : yabai -m window --close
      cmd - f : yabai -m window --toggle float
      cmd - space : yabai -m window --toggle zoom-fullscreen

      # Переключение дисплеев
      cmd - period : yabai -m display --focus next
      cmd - comma : yabai -m display --focus prev

      # Перемещение на другой дисплей
      cmd + shift - period : yabai -m window --display next; yabai -m display --focus next
      cmd + shift - comma : yabai -m window --display prev; yabai -m display --focus prev

      # Перемещение окон
      cmd + shift - h : yabai -m window --swap west
      cmd + shift - j : yabai -m window --swap south
      cmd + shift - k : yabai -m window --swap north
      cmd + shift - l : yabai -m window --swap east

      # Перемещение фокуса
      cmd - h : yabai -m window --focus west
      cmd - j : yabai -m window --focus south
      cmd - k : yabai -m window --focus north
      cmd - l : yabai -m window --focus east

      # Запуск терминала
      cmd - return : open -a "ghostty"

      # Скриншот
      cmd + shift - 4 : screencapture -i -c

      # Дополнительные функции
      cmd + alt - r : yabai -m space --rotate 270
      cmd + alt - x : yabai -m space --mirror x-axis
      cmd + alt - y : yabai -m space --mirror y-axis
      cmd + alt - e : yabai -m space --balance

      # Управление сервисами
      cmd + alt - q : yabai --stop-service
      cmd + alt - s : yabai --start-service
      cmd + alt - r : yabai --restart-service
      cmd + alt - k : skhd --restart-service
    '';
  };
}

