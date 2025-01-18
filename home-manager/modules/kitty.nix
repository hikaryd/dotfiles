{ pkgs, config, ... }: {
  catppuccin.kitty.enable = true;
  programs.kitty = {
    enable = true;
    package = config.lib.nixGL.wrap pkgs.kitty;
    settings = {
      background_opacity = 0.8;

      window_padding_width = 0;
      scrollback_lines = 80000;
      scrollback_pager =
        "less --chop-long-lines --RAW-CONTROL-CHARS +INPUT_LINE_NUMBER";
      cursor_blink_interval = 0;
      enable_audio_bell = "no";
      visual_bell_duration = "0.1";
      window_alert_on_bell = "yes";
      url_style = "curly";
      detect_urls = "yes";
      copy_on_select = "clipboard";
      term = "xterm-256color";
      allow_remote_control = "yes";
      listen_on = "unix:/tmp/kitty";
      shell_integration = "enabled no-cursor";
    };
    font = {
      name = "Maple Mono";
      size = 17;
    };
    keybindings = {
      "ctrl+shift+c" = "copy_to_clipboard";
      "ctrl+shift+v" = "paste_from_clipboard";
      "ctrl+shift+equal" = "change_font_size all +2.0";
      "ctrl+shift+minus" = "change_font_size all -2.0";
      "ctrl+shift+backspace" = "change_font_size all 0";
      "ctrl+shift+e" = "open_url_with_selection";
      "alt+left" = "send_text all \\x1b\\x62";
      "alt+right" = "send_text all \\x1b\\x66";
      "ctrl+a>r" = "load_config_file";
      "ctrl+shift+s>w" =
        "write_config_file ~/.config/kitty/current-session.conf";
      "ctrl+shift+s>r" =
        "load_config_file ~/.config/kitty/current-session.conf";
      "ctrl+a>y" = "focus_visible_window";
    };
  };
}
