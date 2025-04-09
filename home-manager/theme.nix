{ pkgs, inputs, ... }: {
  imports = [ inputs.stylix.homeManagerModules.stylix ];
  home.packages = with pkgs; [
    jetbrains-mono
    noto-fonts
    noto-fonts-emoji
    noto-fonts-cjk-sans
    hack-font
    bemoji
    maple-mono.NF
    nerd-fonts.jetbrains-mono
    iosevka
    open-sans
    monaspace

    bibata-cursors
    base16-schemes
    catppuccin-gtk
    inter
  ];

  fonts.fontconfig.enable = true;

  stylix = {
    enable = true;
    autoEnable = true;
    polarity = "dark";
    base16Scheme = "${pkgs.base16-schemes}/share/themes/catppuccin-mocha.yaml";
    image = ../wallpapers/foggy-city.png;

    fonts = {
      serif = {
        package = pkgs.inter;
        name = "Inter Black Italic";
      };
      sansSerif = {
        package = pkgs.inter;
        name = "Inter Regular";
      };
      monospace = {
        package = pkgs.sf-mono-liga-bin;
        name = "Monaspace Neon Frozen";
      };
      emoji = {
        package = pkgs.noto-fonts-emoji;
        name = "Noto Color Emoji";
      };
    };

    targets = {
      waybar.enable = false;
      firefox.colorTheme.enable = true;
      firefox.firefoxGnomeTheme.enable = true;
      hyprlock.enable = false;
      dunst.enable = false;
      hyprpaper.enable = true;
      qt = {
        enable = true;
        platform = "qtct";
      };
    };
    cursor = {
      size = 20;
      name = "Bibata-Modern-Ice";
      package = pkgs.bibata-cursors;
    };
  };
}
