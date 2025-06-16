{ pkgs, inputs, ... }: {
  imports = [ inputs.stylix.homeModules.stylix ];
  home.packages = with pkgs; [
    noto-fonts
    noto-fonts-emoji
    nerd-fonts.jetbrains-mono
    base16-schemes
    monaspace
  ];

  fonts.fontconfig.enable = true;

  stylix = {
    enable = true;
    autoEnable = true;
    polarity = "dark";
    base16Scheme = "${pkgs.base16-schemes}/share/themes/catppuccin-mocha.yaml";

    fonts = {
      serif = {
        package = pkgs.nerd-fonts.jetbrains-mono;
        name = "Jetbrains Mono";
      };
      sansSerif = {
        package = pkgs.nerd-fonts.jetbrains-mono;
        name = "Jetbrains Mono";
      };
      monospace = {
        package = pkgs.nerd-fonts.jetbrains-mono;
        name = "Jetbrains Mono";
      };
      emoji = {
        package = pkgs.noto-fonts-emoji;
        name = "Noto Color Emoji";
      };

      sizes.terminal = 15;
    };

    targets = {
      waybar.enable = false;
      hyprlock.enable = false;
      dunst.enable = false;
      mako.enable = false;
      starship.enable = false;
      nushell.enable = false;
    };
    opacity.terminal = 0.8;
    cursor = {
      size = 23;
      name = "phinger-cursors-light";
      package = pkgs.phinger-cursors;
    };
  };
}
