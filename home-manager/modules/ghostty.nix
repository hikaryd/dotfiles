{ inputs, pkgs, ... }: {
  home.packages = [
    (pkgs.writeShellScriptBin "ghostty" ''
      ${pkgs.nixgl.nixGLMesa}/bin/nixGLMesa ${
        inputs.ghostty.packages.${pkgs.system}.default
      }/bin/ghostty "$@"
    '')
  ];

  xdg.configFile."ghostty/config".text = ''
    # Font
    font-family = "Maple Mono"
    font-size = 17
    font-thicken = true
    font-feature = ss01
    font-feature = ss04

    bold-is-bright = false
    adjust-box-thickness = 1

    # Theme
    theme = "catppuccin-mocha"
    background-opacity = 0.66

    cursor-style = bar
    cursor-style-blink = true
    adjust-cursor-thickness = 1

    copy-on-select = true
    confirm-close-surface = false
    mouse-hide-while-typing = true

    window-theme = ghostty
    window-padding-x = 4
    window-padding-y = 6
    window-padding-balance = true
    window-padding-color = background
    window-inherit-working-directory = true
    window-inherit-font-size = true
    window-decoration = false

    gtk-titlebar = false
    gtk-single-instance = true
    gtk-wide-tabs = false
  '';
}
