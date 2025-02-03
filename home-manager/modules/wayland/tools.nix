{ config, pkgs, ... }: {
  home.file.".config/kanshi/config".text = ''
    profile HDMI {
      output HDMI-A-1 mode "2560x1440@120" position 0,0 scale 1.25
      output eDP-1 disable
    }

    profile laptop {
      output eDP-1 mode "2560x1600@120" position -2560,0 scale 1.6
    }
  '';

  home.file.".config/swaync/config.toml".text = ''
    [theme]
    name = "catppucin"
    accent_color = "#ff79c6"
    background_color = "#282a36"
    text_color = "#f8f8f2"
  '';

  systemd.user.services.kanshi = {
    Unit = { After = [ "graphical-session.target" ]; };
    Service = {
      ExecStart =
        "${pkgs.kanshi}/bin/kanshi --config ${config.home.homeDirectory}/.config/kanshi/config";
      Restart = "always";
    };
    Install = { WantedBy = [ "default.target" ]; };
  };

  home.packages = with pkgs; [
    (config.lib.nixGL.wrap swaybg)
    (config.lib.nixGL.wrap swaynotificationcenter)
    kanshi
  ];
}

