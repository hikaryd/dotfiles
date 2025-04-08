{ config, pkgs, ... }: {
  services.dunst = {
    enable = true;
    settings = {
      global = {
        browser = "${config.programs.firefox.package}/bin/firefox -new-tab";
        dmenu = "${pkgs.rofi}/bin/rofi -dmenu";
        follow = "mouse";
        format = "<b>%s</b>\\n%b";
        frame_color = "#3c4048";
        frame_width = 2;
        horizontal_padding = 8;
        icon_position = "off";
        line_height = 0;
        markup = "full";
        padding = 8;
        # separator_color = "frame";
        separator_height = 2;
        offset = "0x50";
        notification_limit = 4;
        transparency = 15;
        word_wrap = true;
        corner_radius = 10;
      };
      urgency_low = {
        # background = "#16181a";
        # foreground = "#ffffff";
        # frame_color = "#5eff6c";

        timeout = 10;
      };
      urgency_normal = {
        # background = "#1e2124";
        # foreground = "#ffffff";
        # frame_color = "#5ea1ff";
        timeout = 5;
      };
      urgency_critical = {
        # background = "#16181a";
        # foreground = "#ff6e5e";
        # frame_color = "#f1ff5e";
        timeout = 30;
      };
      shortcuts = {
        context = "mod4+grave";
        close = "mod4+shift+space";
      };
    };
  };
}
