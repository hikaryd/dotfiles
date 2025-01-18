{ pkgs, lib, config, ... }: {
  xdg.configFile."hypr/hyprpaper.conf".text = ''
    preload = ${config.stylix.image}
    wallpaper = , ${config.stylix.image}
  '';

  systemd.user.services.hyprpaper = {
    Service = { ExecStart = "${lib.getExe pkgs.hyprpaper}"; };
    Install.WantedBy = [ "graphical-session.target" ];
  };
}
