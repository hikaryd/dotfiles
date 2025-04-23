{ pkgs, ... }: {
  home.packages = with pkgs; [ waybar ];
  home.file.".config/waybar/" = {
    source = ../../configs/waybar;
    recursive = true;
  };
}
