{ pkgs, config, ... }: {
  imports = [ ./conf.nix ./hypridle.nix ./hyprpaper.nix ./hyprpanel.nix ];

  home.packages = with pkgs; [
    xdg-desktop-portal-hyprland
    wofi
    hypridle
    grimblast
    gtk3
    glib
    libdbusmenu-gtk3
    wlroots
    xwayland
    libinput
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
    xwayland.enable = true;
    systemd = {
      enable = true;
      variables = [ ];
    };
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
    "wofi" = {
      source = ./wofi;
      recursive = true;
    };
  };
}
