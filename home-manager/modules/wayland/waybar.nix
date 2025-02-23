{ pkgs, inputs, ... }: {
  home.packages = with pkgs; [
    waybar
    inputs.master.legacyPackages.${system}.pamixer
  ];
  home.file.".config/waybar/" = {
    source = ../../configs/waybar;
    recursive = true;
  };
}
