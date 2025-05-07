{ pkgs, inputs, ... }: {
  imports = [ inputs.stylix.homeManagerModules.stylix ];
  home.packages = with pkgs; [
    noto-fonts
    noto-fonts-emoji
    nerd-fonts.jetbrains-mono
    base16-schemes
  ];

  fonts.fontconfig.enable = true;

  stylix = {
    enable = true;
    autoEnable = true;
    polarity = "dark";
    base16Scheme = "${pkgs.base16-schemes}/share/themes/catppuccin-mocha.yaml";
    image = ../wallpapers/keyboard.png;

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

      sizes.terminal = 13;
    };

    targets = {
      waybar.enable = false;
      hyprlock.enable = false;
      dunst.enable = false;
      mako.enable = false;
    };
    cursor = {
      size = 23;
      name = "phinger-cursors-light";
      package = pkgs.phinger-cursors;
    };
  };
}
