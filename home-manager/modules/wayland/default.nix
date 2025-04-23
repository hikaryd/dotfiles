{ pkgs, ... }: {
  imports = [ ./hyprland ./anyrun ./dunst.nix ./niri.nix ./waybar.nix ];

  home.packages = with pkgs; [ glib wlroots xwayland libinput ];
}
