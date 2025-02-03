{ pkgs, config, ... }: {
  imports = [ ./hyprland ./anyrun ./hyprpanel.nix ./tools.nix ];

  home.packages = with pkgs; [
    glib
    wlroots
    xwayland
    libinput
    (config.lib.nixGL.wrap xdg-desktop-portal)
    (config.lib.nixGL.wrap xdg-desktop-portal-wlr)
    (config.lib.nixGL.wrap xdg-desktop-portal-gtk)
  ];
}
