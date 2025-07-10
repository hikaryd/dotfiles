{ pkgs, ... }:
let sketchybar = pkgs.sketchybar;
in {
  home.packages = [ sketchybar ];

  home.file.".config/sketchybar" = {
    source = ../configs/sketchybar;
    recursive = false;
  };
  #
  # launchd.agents.sketchybar = {
  #   enable = true;
  #   config = {
  #     Label = "sketchybar";
  #     ProgramArguments = [ "${sketchybar}/bin/sketchybar" ];
  #     RunAtLoad = true;
  #     KeepAlive = true;
  #   };
  # };
}
