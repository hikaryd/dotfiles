{ pkgs, inputs, ... }: {
  imports = [ inputs.stylix.homeModules.stylix ];
  home.packages = with pkgs; [
    noto-fonts
    noto-fonts-emoji
    nerd-fonts.jetbrains-mono
    base16-schemes
    monaspace
    fira-code
  ];

  fonts.fontconfig.enable = true;

  stylix = {
    enable = true;
    autoEnable = true;
    polarity = "dark";
    base16Scheme = "${pkgs.base16-schemes}/share/themes/catppuccin-mocha.yaml";

    targets = {
      starship.enable = false;
      nushell.enable = false;
      ghostty.enable = false;
      helix.enable = false;
    };
  };
}
