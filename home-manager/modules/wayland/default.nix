{ pkgs, ... }: {
  imports = [ ./hyprland ./anyrun ./dunst.nix ./niri.nix ];

  home.packages = with pkgs; [ glib wlroots xwayland libinput ];
}
