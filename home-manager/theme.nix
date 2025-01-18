{ pkgs, ... }: {
  home.packages = with pkgs; [
    jetbrains-mono
    noto-fonts
    noto-fonts-emoji
    noto-fonts-cjk-sans
    bemoji
    maple-mono
    nerd-fonts.jetbrains-mono

    bibata-cursors
    base16-schemes
    catppuccin-gtk
  ];

  fonts.fontconfig.enable = true;

  stylix = {
    enable = true;
    base16Scheme = "${pkgs.base16-schemes}/share/themes/catppuccin-mocha.yaml";
    image = ../wallpapers/hollow-knight.jpg;

    fonts = {
      serif = {
        package = pkgs.jetbrains-mono;
        name = "JetBrains Mono";
      };
      sansSerif = {
        package = pkgs.jetbrains-mono;
        name = "JetBrains Mono";
      };
      monospace = {
        package = pkgs.jetbrains-mono;
        name = "JetBrains Mono";
      };
      emoji = {
        package = pkgs.noto-fonts-emoji;
        name = "Noto Color Emoji";
      };
    };

    targets = {
      gtk.enable = true;
      kitty.enable = false;
      bat.enable = false;
      hyprlock.enable = true;
      hyprpaper.enable = true;
    };
    cursor = {
      size = 20;
      name = "Bibata-Modern-Ice";
      package = pkgs.bibata-cursors;
    };
  };
}

