{ pkgs, config, ... }: {
  imports = [
    ./hyprland
    ./anyrun
    ./tools.nix
    ./waybar.nix
    ./dunst.nix
    ./hyprpanel.nix
  ];

  home.packages = with pkgs; [
    glib
    wlroots
    xwayland
    libinput
    sway-launcher-desktop
    (config.lib.nixGL.wrap xdg-desktop-portal)
    (config.lib.nixGL.wrap xdg-desktop-portal-wlr)
    (config.lib.nixGL.wrap xdg-desktop-portal-gtk)
  ];
}
