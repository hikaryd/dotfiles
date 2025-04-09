{ ... }:

let
  catppuccin = {
    rosewater = "#f5e0dc";
    flamingo = "#f2cdcd";
    pink = "#f5c2e7";
    mauve = "#cba6f7";
    red = "#f38ba8";
    maroon = "#eba0ac";
    peach = "#fab387";
    yellow = "#f9e2af";
    green = "#a6e3a1";
    teal = "#94e2d5";
    sky = "#89dceb";
    sapphire = "#74c7ec";
    blue = "#89b4fa";
    lavender = "#b4befe";
    text = "#cdd6f4";
    subtext1 = "#bac2de";
    subtext0 = "#a6adc8";
    overlay2 = "#9399b2";
    overlay1 = "#7f849c";
    overlay0 = "#6c7086";
    surface2 = "#585b70";
    surface1 = "#45475a";
    surface0 = "#313244";
    base = "#1e1e2e";
    mantle = "#181825";
    crust = "#11111b";
  };
in {
  services.dunst = {
    enable = true;
    settings = {
      global = {
        font = "JetBrainsMono Nerd Font Medium 10";
        format = ''
          %I <b>%s</b>
          %b'';

        markup = "full";
        word_wrap = true;
        alignment = "left";
        vertical_alignment = "center";

        # --- Geometry & Positioning ---
        geometry = "300x5-30+60";
        origin = "top_right";
        notification_limit = 5;

        # --- Padding & Frame ---
        padding = 10;
        horizontal_padding = 10;
        frame_width = 2;
        separator_height = 1;
        corner_radius = 12;

        # --- Colors & Transparency ---
        background = catppuccin.base;
        foreground = catppuccin.text;
        frame_color = catppuccin.surface0;
        separator_color = catppuccin.surface0;
        transparency = 15;

        # --- Icons ---
        icon_position = "left";
        max_icon_size = 32;

        # --- Behavior ---
        follow = "mouse";
        sticky_history = true;
        history_length = 20;
        show_indicators = false;
        line_height = 0;
        shrink = "no";
        sort = "yes";

        # --- Mouse & Keyboard ---
        browser = "google-chrome-stable";
        dmenu = "anyrun";
        mouse_left_click = "close_current";
        mouse_middle_click = "do_action";
        mouse_right_click = "close_all";
      };

      # --- Urgency Levels ---
      urgency_low = {
        background = catppuccin.base;
        foreground = catppuccin.text;
        frame_color = catppuccin.green;
        timeout = 5;
      };
      urgency_normal = {
        background = catppuccin.base;
        foreground = catppuccin.text;
        frame_color = catppuccin.blue;
        timeout = 10;
      };
      urgency_critical = {
        background = catppuccin.base;
        foreground = catppuccin.red;
        frame_color = catppuccin.red;
        timeout = 0;
      };

      # --- Shortcuts (Сохранены твои) ---
      shortcuts = {
        context = "mod4+grave";
        close = "mod4+shift+space";
        close_all = "mod4+ctrl+space";
        history = "mod4+h";
        do_action = "mod4+a";
      };
    };
  };
}
