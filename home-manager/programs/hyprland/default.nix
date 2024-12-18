{ pkgs, config, ... }: {
  imports = [ ./conf.nix ./hyprpaper.nix ./hyprlock.nix ./hypridle.nix ];

  home.file = {
    ".wallpapers".source = config.lib.file.mkOutOfStoreSymlink
      "${config.home.homeDirectory}/dotfiles/wallpapers";
  };

  xdg.dataFile."wayland-sessions/hyprland.desktop".text = ''
    [Desktop Entry]
    Name=Hyprland
    Comment=An intelligent dynamic tiling Wayland compositor
    Exec=Hyprland
    Type=Application
  '';

  wayland.windowManager.hyprland = {
    enable = true;
    xwayland.enable = true;
  };

  home.packages = with pkgs; [ 
    xdg-desktop-portal-hyprland
    hyprpaper 
    wofi 
    hypridle 
    hyprlock 
    grimblast 
    hyprland
  ];
}
