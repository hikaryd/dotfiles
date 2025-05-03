{ pkgs, inputs, lib, ... }: {
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
    fira-code

    base16-schemes
    catppuccin-gtk
    inter
  ];

  fonts.fontconfig.enable = true;

  stylix = {
    enable = true;
    autoEnable = true;
    polarity = "dark";
    base16Scheme = "${pkgs.base16-schemes}/share/themes/tokyodark.yaml";
    image = ../wallpapers/keyboard.png;

    fonts = lib.mkIf (!pkgs.stdenv.isDarwin) {
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
    };

    targets = {
      waybar.enable = false;
      firefox.colorTheme.enable = true;
      firefox.firefoxGnomeTheme.enable = true;
      hyprlock.enable = false;
      dunst.enable = false;
      hyprpaper.enable = true;
      qt = {
        enable = if !pkgs.stdenv.isDarwin then true else false;
        platform = "qtct";
      };
    };
    cursor = {
      size = 23;
      name = "phinger-cursors-light";
      package = pkgs.phinger-cursors;
    };
  };
}
