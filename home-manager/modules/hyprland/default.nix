{ pkgs, inputs, ... }: {
  imports = [
    ./config.nix
    ./hypridle.nix
    ./hyprpanel.nix
    inputs.hyprland.homeManagerModules.default
  ];

  home.packages = with pkgs; [
    (pkgs.writeShellScriptBin "hypridle" ''
      ${pkgs.nixgl.nixGLMesa}/bin/nixGLMesa ${hypridle}/bin/hypridle "$@"
    '')
    (pkgs.writeShellScriptBin "grimblast" ''
      ${pkgs.nixgl.nixGLMesa}/bin/nixGLMesa ${grimblast}/bin/grimblast "$@"
    '')
    (pkgs.writeShellScriptBin "start-hyprland" ''
      ${pkgs.nixgl.nixGLMesa}/bin/nixGLMesa ${
        inputs.hyprland.packages.${pkgs.system}.default
      }/bin/Hyprland
    '')

    xdg-desktop-portal-hyprland
    glib
    wlroots
    grim
    xwayland
    libinput
  ];

  home.file.".local/share/wayland-sessions/hyprland.desktop".text = ''
    [Desktop Entry]
    Name=Hyprland
    Comment=An intelligent dynamic tiling Wayland compositor
    Exec=/home/hikary/.nix-profile/bin/start-hyprland
    Type=Application
    DesktopNames=Hyprland
  '';

  xdg.configFile."hypr/hyprpaper.conf".text = ''
    preload = ${../../../wallpapers/hollow-knight.jpg}
    wallpaper = ,${../../../wallpapers/hollow-knight.jpg}
    ipc = off
  '';
}
