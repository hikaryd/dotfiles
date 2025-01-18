{ pkgs, ... }: {
  imports = [
    ./niri
    ./hyprland
    ./anyrun
    ./hyprpanel.nix
    ./hypridle.nix
    ./hyprpaper.nix
    ./hyprlock.nix
  ];

  home.packages = with pkgs; [ glib wlroots xwayland libinput ];
}
