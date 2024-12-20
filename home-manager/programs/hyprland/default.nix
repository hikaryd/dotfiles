{ pkgs, config, ... }:

let
  flake-compat = builtins.fetchTarball {
    url = "https://github.com/edolstra/flake-compat/archive/master.tar.gz";
  };

  hyprpanel = (import flake-compat {
    src = builtins.fetchTarball {
      url = "https://github.com/Jas-SinghFSU/HyprPanel/archive/master.tar.gz";
    };
  }).defaultNix;

  hyprpanelPkg = (hyprpanel.overlay pkgs pkgs).hyprpanel;
in {
  imports = [ ./conf.nix ./hyprpaper.nix ./hypridle.nix ];

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
    xwayland.enable = true;
  };

  home.packages = with pkgs; [
    xdg-desktop-portal-hyprland
    wofi
    hypridle
    grimblast
    hyprland
    gtk3
    glib
    libdbusmenu-gtk3
    hyprpanelPkg
  ];

  xdg.configFile = {
    "hyprpanel" = {
      source = ./hyprpanel;
      recursive = true;
    };
    "wofi" = {
      source = ./wofi;
      recursive = true;
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
}
