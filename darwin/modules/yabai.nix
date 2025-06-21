{ config, pkgs, ... }:
let
  user = config.system.primaryUser;

  switchToSpaceScript = pkgs.writeShellScript "switch_to_space.sh" # bash
    ''
      #!/bin/bash
      SPACE=$1
      CURRENT_SPACE=$(${pkgs.yabai}/bin/yabai -m query --spaces --space | ${pkgs.jq}/bin/jq .index)

      echo $CURRENT_SPACE > ~/.config/yabai/.previous_space

      case $SPACE in
        1) KEY=18 ;;
        2) KEY=19 ;;
        3) KEY=20 ;;
        4) KEY=21 ;;
        5) KEY=23 ;;
        6) KEY=22 ;;
        7) KEY=26 ;;
        8) KEY=28 ;;
        *) exit 1 ;;
      esac

      osascript -e "tell application \"System Events\" to key code $KEY using {control down, command down}"
    '';

  switchToRecentSpaceScript =
    pkgs.writeShellScript "switch_to_recent_space.sh" # bash
    ''
      #!/bin/bash

      if [ -f ~/.config/yabai/.previous_space ]; then
        PREVIOUS_SPACE=$(cat ~/.config/yabai/.previous_space)
      else
        PREVIOUS_SPACE=1
      fi

      CURRENT_SPACE=$(${pkgs.yabai}/bin/yabai -m query --spaces --space | ${pkgs.jq}/bin/jq .index)

      if [ "$CURRENT_SPACE" -eq "$PREVIOUS_SPACE" ]; then
        exit 0
      fi

      ${switchToSpaceScript} $PREVIOUS_SPACE
    '';

  createSpacesScript = pkgs.writeShellScript "create_spaces.sh" # bash
    ''
      #!/bin/bash

      CURRENT_SPACES=$(${pkgs.yabai}/bin/yabai -m query --spaces | ${pkgs.jq}/bin/jq length)

      if [ $CURRENT_SPACES -lt 8 ]; then
        echo "Создание spaces... Текущее количество: $CURRENT_SPACES"
        
        osascript -e 'tell application "Mission Control" to launch'
        sleep 1
        
        for ((i=$CURRENT_SPACES; i<8; i++)); do
          osascript -e 'tell application "System Events" to click (every button whose value of attribute "AXDescription" is "add desktop") of UI element "Spaces Bar" of UI element 1 of group 1 of process "Dock"'
          sleep 0.5
        done
        
        osascript -e 'tell application "System Events" to key code 53'  # ESC
        
        echo "Spaces созданы!"
      else
        echo "Уже есть $CURRENT_SPACES spaces"
      fi
    '';

in {
  environment.systemPackages = with pkgs; [ jq ];

  system.defaults.spaces.spans-displays = true; # Displays have separate Spaces
  system.defaults.dock.mru-spaces =
    false; # НЕ переставлять spaces автоматически

  services = {
    yabai = {
      enable = true;
      package = pkgs.yabai;
      enableScriptingAddition = false; # Оставляем false для работы с SIP
      config = {
        layout = "bsp";
        window_gap = 12;
        top_padding = 20;
        bottom_padding = 20;
        left_padding = 20;
        right_padding = 20;
        window_placement = "second_child";
        focus_follows_mouse = "autofocus";
        mouse_follows_focus = "off";
        mouse_modifier = "alt";
        mouse_action1 = "move";
        mouse_action2 = "resize";
        mouse_drop_action = "swap";
      };
      extraConfig = # bash
        ''
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

          # yabai -m rule --add app="^Ghostty$" manage=off
          yabai -m signal --add app='^Ghostty$' event=window_created   action='yabai -m space --layout bsp'
          yabai -m signal --add app='^Ghostty$' event=window_destroyed action='yabai -m space --layout bsp'
        '';
    };

    skhd = {
      enable = true;
      package = pkgs.skhd;
      skhdConfig = # bash
        ''
          ################  ФОКУС ОКНА (hjkl)  ################
          alt - h : yabai -m window --focus west  || yabai -m display --focus west
          alt - j : yabai -m window --focus south || yabai -m display --focus south
          alt - k : yabai -m window --focus north || yabai -m display --focus north
          alt - l : yabai -m window --focus east  || yabai -m display --focus east

          ################  ПЕРЕМЕЩЕНИЕ ОКОН  ################
          shift + alt - h : yabai -m window --swap west
          shift + alt - j : yabai -m window --swap south
          shift + alt - k : yabai -m window --swap north
          shift + alt - l : yabai -m window --swap east

          ################  RESIZE  ################
          shift + alt - minus : yabai -m window --resize left:-20:0 || yabai -m window --resize right:-20:0
          shift + alt - equal : yabai -m window --resize left:20:0 || yabai -m window --resize right:20:0

          ################  ПОЛНОЭКРАННЫЙ РЕЖИМ  ################
          alt + shift - f : yabai -m window --toggle zoom-fullscreen

          ################  БАЛАНС РАЗМЕРОВ  ################
          alt + shift - 0 : yabai -m space --balance

          ################  ЗАПУСК GHOSTTY  ################
          alt - return : open -na Ghostty

          ################  ПЕРЕМЕЩЕНИЕ МЕЖДУ ДИСПЛЕЯМИ  ################
          ctrl + alt - h : yabai -m window --display west  && yabai -m display --focus west
          ctrl + alt - l : yabai -m window --display east  && yabai -m display --focus east

          ################  ФОКУС ДИСПЛЕЕВ  ################
          cmd + shift - l : yabai -m display --focus next
          cmd + shift - h : yabai -m display --focus prev

          ################  ПЕРЕКЛЮЧЕНИЕ НА ПРЕДЫДУЩИЙ SPACE  ################
          alt - tab : ${switchToRecentSpaceScript}

          ################  ПЕРЕКЛЮЧЕНИЕ НА КОНКРЕТНЫЕ SPACE  ################
          alt - 1 : ${switchToSpaceScript} 1
          alt - 2 : ${switchToSpaceScript} 2
          alt - 3 : ${switchToSpaceScript} 3
          alt - 4 : ${switchToSpaceScript} 4
          alt - 5 : ${switchToSpaceScript} 5
          alt - 6 : ${switchToSpaceScript} 6
          alt - 7 : ${switchToSpaceScript} 7
          alt - 8 : ${switchToSpaceScript} 8

          # Перемещение окон на space
          alt + shift - 1 : yabai -m window --space 1
          alt + shift - 2 : yabai -m window --space 2
          alt + shift - 3 : yabai -m window --space 3
          alt + shift - 4 : yabai -m window --space 4
          alt + shift - 5 : yabai -m window --space 5
          alt + shift - 6 : yabai -m window --space 6
          alt + shift - 7 : yabai -m window --space 7
          alt + shift - 8 : yabai -m window --space 8

          # Перемещение space на другой монитор
          alt + shift - tab : yabai -m space --display next || yabai -m space --display first

          # Переключение между floating и tiling
          cmd + shift - f : yabai -m window --toggle float

          # Закрытие окна
          alt - q : yabai -m window --close

          # Перезапуск yabai
          ctrl + alt + cmd - r : yabai --restart-service

          # Создание spaces
          ctrl + alt + cmd - n : ${createSpacesScript}
        '';
    };
  };

  system.activationScripts.yabaiSetup = {
    text = # bash
      ''
        echo "Настройка yabai..."

        mkdir -p /Users/${user}/.config/yabai

        ln -sf ${switchToSpaceScript} /Users/${user}/.config/yabai/switch_to_space.sh
        ln -sf ${switchToRecentSpaceScript} /Users/${user}/.config/yabai/switch_to_recent_space.sh
        ln -sf ${createSpacesScript} /Users/${user}/.config/yabai/create_spaces.sh

        echo "Настройка хоткеев Mission Control..."

        set_mission_control_key() {
          local space=$1
          local keycode=$2
          local action_id=$((117 + $space))  # 118 для space 1, 119 для space 2 и т.д.
          
          # Настройка хоткея через defaults write
          # Формат: ctrl+cmd+[число]
          # parameters: (65535, keycode, 1310720)
          # 1310720 = Control + Command
          sudo -u ${user} defaults write com.apple.symbolichotkeys AppleSymbolicHotKeys -dict-add $action_id "
          <dict>
            <key>enabled</key><true/>
            <key>value</key><dict>
              <key>type</key><string>standard</string>
              <key>parameters</key>
              <array>
                <integer>65535</integer>
                <integer>$keycode</integer>
                <integer>1310720</integer>
              </array>
            </dict>
          </dict>
          "
        }

        set_mission_control_key 1 18  # keycode 18 = 1
        set_mission_control_key 2 19  # keycode 19 = 2
        set_mission_control_key 3 20  # keycode 20 = 3
        set_mission_control_key 4 21  # keycode 21 = 4
        set_mission_control_key 5 23  # keycode 23 = 5
        set_mission_control_key 6 22  # keycode 22 = 6
        set_mission_control_key 7 26  # keycode 26 = 7
        set_mission_control_key 8 28  # keycode 28 = 8

        /System/Library/PrivateFrameworks/SystemAdministration.framework/Resources/activateSettings -u 2>/dev/null || true

        echo ""
        echo "========================================="
        echo "Настройка yabai завершена!"
        echo ""
        echo "Важно: для полной работы необходимо:"
        echo "1. Перезагрузить систему для применения хоткеев Mission Control"
        echo "2. Запустить 'ctrl+alt+cmd+n' для создания 8 spaces"
        echo "3. Дать разрешения skhd в System Preferences > Security & Privacy > Accessibility"
        echo "========================================="
      '';
  };
}
