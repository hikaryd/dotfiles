{ pkgs, lib, ... }: {
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
    image = ../wallpapers/bsod.png;

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
    };
  };

  home.pointerCursor = {
    package = lib.mkForce pkgs.bibata-cursors;
    name = lib.mkForce "Bibata-Modern-Ice";
    size = lib.mkForce 20;
    gtk.enable = lib.mkForce true;
    x11.enable = lib.mkForce true;
  };
}
