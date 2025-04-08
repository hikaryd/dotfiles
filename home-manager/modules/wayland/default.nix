{ pkgs, ... }: {
  imports = [
    ./hyprland
    ./anyrun
    ./tools.nix
    ./waybar.nix
    ./dunst.nix
    ./hyprpanel.nix
    ./niri.nix
  ];

  home.packages = with pkgs; [ glib wlroots xwayland libinput ];
}
