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
    hyprpaper
    inter
  ];

  fonts.fontconfig.enable = true;

  stylix = {
    enable = true;
    autoEnable = true;
    polarity = "dark";
    base16Scheme = builtins.fetchurl {
      url =
        "https://raw.githubusercontent.com/scottmckendry/cyberdream.nvim/main/extras/base16/cyberdream.yaml";
      sha256 = "1bfi479g7v5cz41d2s0lbjlqmfzaah68cj1065zzsqksx3n63znf";
    };
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
        package = pkgs.monaspace;
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
    };
    cursor = {
      size = 20;
      name = "Bibata-Modern-Ice";
      package = pkgs.bibata-cursors;
    };
  };
}
