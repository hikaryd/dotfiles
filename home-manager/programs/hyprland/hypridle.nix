{ ... }: {
  services.hypridle = {
    enable = true;
    settings = {
      general = {
        lock_cmd = "";
        before_sleep_cmd = "hyprctl dispatch dpms off";
        after_sleep_cmd = "hyprctl dispatch dpms on";
        ignore_dbus_inhibit = false;
      };

      listener = [{
        timeout = 600;
        on-timeout = "loginctl suspend";
      }];
    };
  };
}
