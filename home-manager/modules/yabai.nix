{ ... }: {
  home.file.".yabairc".text = # bash
    ''
      yabai -m config layout               bsp
      yabai -m config auto_balance         on
      yabai -m config window_gap           8
      yabai -m config top_padding          20
      yabai -m config bottom_padding       20
      yabai -m config left_padding         20
      yabai -m config right_padding        20

      yabai -m config window_border        off

      # yabai -m rule --add app="^Slack$"          manage=on
      # yabai -m rule --add app="^Google Chrome$"  manage=on

      borders active_color=0xffcba6f7 \
              inactive_color=0xff45475a \
              width=6 style=round &
    '';
  home.file.".skhdrc".text = # bash
    ''
      spf() { osascript -e "tell application \"System Events\" \
                to key code $1 using {control down$2}" ; }

      # ---------- Space — прыжок ----------
      alt - 1 : spf 18        # Desktop 1
      alt - 2 : spf 19
      alt - 3 : spf 20
      alt - 4 : spf 21
      alt - 5 : spf 23
      alt - 6 : spf 22
      alt - 7 : spf 26
      alt - 8 : spf 28        # Desktop 8

      # ---------- Space — отправить окно и перейти ----------
      alt + shift - 1 : spf 18 ", shift down"
      alt + shift - 2 : spf 19 ", shift down"
      alt + shift - 3 : spf 20 ", shift down"
      alt + shift - 4 : spf 21 ", shift down"
      alt + shift - 5 : spf 23 ", shift down"
      alt + shift - 6 : spf 22 ", shift down"
      alt + shift - 7 : spf 26 ", shift down"
      alt + shift - 8 : spf 28 ", shift down"

      # ---------- фокус внутри Space ----------
      alt - h : yabai -m window --focus west
      alt - j : yabai -m window --focus south
      alt - k : yabai -m window --focus north
      alt - l : yabai -m window --focus east

      # ---------- перестановка плиток ----------
      alt + shift - h : yabai -m window --swap west
      alt + shift - j : yabai -m window --swap south
      alt + shift - k : yabai -m window --swap north
      alt + shift - l : yabai -m window --swap east

      # ---------- окна / дисплеи ----------
      alt - f       : yabai -m window --toggle zoom-fullscreen
      alt - space   : yabai -m window --toggle float
      cmd - q       : yabai -m window --close
      alt - period  : yabai -m window --display next
      alt - comma   : yabai -m window --display prev

      # ---------- запуск Ghostty по хоткею ----------
      cmd - return  : open -na "ghostty"
    '';
}
