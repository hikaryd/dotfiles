{ pkgs, inputs, config, ... }: {
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
    xwayland
    libinput
  ];

  wayland.windowManager.hyprland = {
    enable = true;
    xwayland.enable = true;
    systemd.enable = true;
  };

  home.file.".local/share/wayland-sessions/hyprland.desktop".text = ''
    [Desktop Entry]
    Name=Hyprland
    Comment=An intelligent dynamic tiling Wayland compositor
    Exec=/home/hikary/.nix-profile/bin/start-hyprland
    Type=Application
    DesktopNames=Hyprland
  '';

  xdg.configFile."hypr/hyprpaper.conf".text = ''
    preload = ${config.home.homeDirectory}/dotfiles/wallpapers/minimalist-moon-night-mountains.jpg
    wallpaper = ,${config.home.homeDirectory}/dotfiles/wallpapers/minimalist-moon-night-mountains.jpg
    ipc = off
  '';
}
