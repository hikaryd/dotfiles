{ pkgs, ... }: {
  home.packages = with pkgs; [
    jetbrains-mono
    noto-fonts
    noto-fonts-emoji
    noto-fonts-cjk-sans
    hack-font
    bemoji
    maple-mono
    nerd-fonts.jetbrains-mono
    iosevka

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
    image = ../wallpapers/cat-vibin.png;

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
        package = pkgs.inter;
        name = "Inter Medium";
      };
      emoji = {
        package = pkgs.noto-fonts-emoji;
        name = "Noto Color Emoji";
      };
    };

    targets = {
      dunst.enable = false;
      wpaperd.enable = false;
      vscode.enable = false;
      waybar.enable = false;
    };
    cursor = {
      size = 20;
      name = "Bibata-Modern-Ice";
      package = pkgs.bibata-cursors;
    };
  };
}
