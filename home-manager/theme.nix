{ pkgs, lib, ... }: {
  home.packages = with pkgs; [
    jetbrains-mono
    noto-fonts
    noto-fonts-emoji
    nerd-fonts.jetbrains-mono

    bibata-cursors
    base16-schemes
  ];

  fonts.fontconfig.enable = true;

  stylix = {
    enable = true;
    base16Scheme = "${pkgs.base16-schemes}/share/themes/catppuccin-mocha.yaml";
    image = ../wallpapers/birdandcat.jpg;

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
      kitty.enable = true;
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
