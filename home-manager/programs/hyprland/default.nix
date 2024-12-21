{ pkgs, config, ... }: {
  imports = [ ./conf.nix ./hyprpaper.nix ./hypridle.nix ./hyprpanel ];

  home.packages = with pkgs; [
    xdg-desktop-portal-hyprland
    wofi
    hypridle
    grimblast
    hyprland
    gtk3
    glib
    libdbusmenu-gtk3
  ];

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
    package = pkgs.hyprland;
    systemd = {
      enable = true;
      variables = [ ];
    };
    xwayland.enable = true;
  };

  xdg.portal = {
    enable = true;
    extraPortals = [ pkgs.xdg-desktop-portal-hyprland ];
    config = {
      common = { default = [ "*" ]; };
      hyprland = {
        default = [ "hyprland" ];
        "org.freedesktop.impl.portal.Screenshot" = [ "hyprland" ];
        "org.freedesktop.impl.portal.Screencast" = [ "hyprland" ];
      };
    };
  };

  xdg.configFile = {
    "hyprpanel" = {
      source = ./hyprpanel;
      recursive = true;
    };
  };
}
