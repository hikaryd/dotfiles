{ ... }:
let
  lock = "pidof /usr/bin/hyprlock || /usr/bin/hyprlock";
  lockWarning = 30;
  lockTimeout = 60 * 5;
  suspendTimeout = 60 * 10;
in {
  services.hypridle = {
    enable = true;
    settings = {
      general = {
        lock_cmd = lock;
        before_sleep_cmd = "loginctl lock-session";
        after_sleep_cmd =
          "killall ax-shell || true && sleep 1 && uwsm app -- /usr/bin/python ~/.config/Ax-Shell/main.py && disown &&  hyprctl dispatch dpms on";
      };
      listener = [
        {
          timeout = lockTimeout - lockWarning;
          on-timeout = "notify-send -t 3000 'Locking' 'This computer locks in ${
              toString lockWarning
            } seconds'";
        }
        {
          timeout = lockTimeout;
          on-timeout = lock;
        }
        {
          timeout = lockTimeout + 30;
          on-timeout = "hyprctl dispatch dpms off";
          on-resume = "hyprctl dispatch dpms on";
        }
        {
          timeout = suspendTimeout;
          on-timeout = "systemctl suspend";
        }
      ];
    };
  };
}
