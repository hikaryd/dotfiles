{ config, pkgs, ... }: {
  imports = [ ./config.nix ./hyprlock.nix ./hypridle.nix ];

  home.packages = with pkgs; [
    (config.lib.nixGL.wrap hypridle)
    (config.lib.nixGL.wrap grimblast)
    grim
  ];
}
