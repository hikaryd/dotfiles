{ pkgs, ... }: {
  home.packages = with pkgs; [ waybar pamixer ];
  home.file.".config/waybar/" = {
    source = ../../configs/waybar;
    recursive = true;
  };
}
