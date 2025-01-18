{ config, pkgs, ... }: {
  imports = [ ./config.nix ];

  home.packages = with pkgs; [
    (config.lib.nixGL.wrap hypridle)
    (config.lib.nixGL.wrap grimblast)
    xdg-desktop-portal-hyprland
    grim
  ];
  home.file.".local/share/wayland-sessions/hyprland.desktop".text = ''
    [Desktop Entry]
    Name=Hyprland-arch
    Comment=An intelligent dynamic tiling Wayland compositor
    Exec=bash -c /usr/bin/Hyprland
    Type=Application
    DesktopNames=Hyprland
  '';
}
