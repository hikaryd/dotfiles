{ pkgs, ... }:

let
  lockCmd = "/usr/bin/hyprlock";
  notifyCmd = "${pkgs.libnotify}/bin/notify-send";
  hyprctlCmd = "hyprctl";
  systemctlCmd = "systemctl";

  lockWarning = 30; # seconds
  lockTimeout = 60 * 5; # 5 minutes
  suspendTimeout = 60 * 10; # 10 minutes

in {
  services.hypridle = {
    enable = false;
    settings = {
      general = {
        lock_cmd = lockCmd;
        after_sleep_cmd = "${hyprctlCmd} dispatch dpms on";
      };

      listener = [
        {
          timeout = lockTimeout - lockWarning; # 270
          on-timeout =
            "${notifyCmd} -t 3000 'Locking' 'Компьютер заблокируется через ${
              toString lockWarning
            } секунд'";
        }
        {
          timeout = lockTimeout;
          on-timeout = lockCmd;
        }
        {
          timeout = suspendTimeout;
          on-timeout = "${systemctlCmd} suspend";
        }
      ];
    };
  };
}

