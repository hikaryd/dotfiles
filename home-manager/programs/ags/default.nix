{ pkgs, ... }:
let
  flake-compat = builtins.fetchTarball "https://github.com/edolstra/flake-compat/archive/master.tar.gz";

  ags-module = (import flake-compat {
    src = builtins.fetchTarball "https://github.com/Aylur/ags/archive/master.tar.gz";
  }).defaultNix;
in {
  imports = [ ags-module.homeManagerModules.default ];

  programs.ags = {
    enable = true;
    configDir = ./config;
    package = ags-module.packages.${pkgs.system}.default.override {
      extraPackages = with pkgs; [
        gtksourceview
        webkitgtk
        accountsservice
        gjs
        networkmanager
        gvfs
        libdbusmenu-gtk3
        gtk3
        gnome.adwaita-icon-theme
        upower
        glib
        gobject-introspection
        pango
        cairo
        gdk-pixbuf
        atk
        at-spi2-core
        dbus
        dconf
        glibc
        swww
        brightnessctl
        grimblast
        gtk-layer-shell
        xdg-user-dirs
        astal
      ];
    };
  };
}
