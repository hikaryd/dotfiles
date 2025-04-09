{ pkgs, ... }: {
  imports =
    [ ./hyprland ./anyrun ./tools.nix ./dunst.nix ./hyprpanel.nix ./niri.nix ];

  home.packages = with pkgs; [ glib wlroots xwayland libinput ];
}
