{ pkgs, ... }:
let
  flake-compat = builtins.fetchTarball {
    url = "https://github.com/edolstra/flake-compat/archive/master.tar.gz";
    sha256 = "sha256:19d2z6xsvpxm184m41qrpi1bplilwipgnzv9jy17fgw421785q1m";
  };
  hyprpanel-source = pkgs.fetchFromGitHub {
    owner = "Jas-SinghFSU";
    repo = "HyprPanel";
    rev = "f21d70949f9f4426f39d12f542ec788d47330763";
    hash = "sha256-RIVP0MVk/LrHYS6DrIuPLAe4Rk7fWf1HbRcWS87a+zA=";
  };
  hyprpanel-flake =
    (import flake-compat { src = hyprpanel-source; }).defaultNix;
in {
  nixpkgs.overlays = [
    (self: super: {
      hyprpanel = hyprpanel-flake.packages.${pkgs.system}.default;
    })
  ];

  home.packages = [ pkgs.hyprpanel ];
}
