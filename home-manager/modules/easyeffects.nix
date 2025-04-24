{ pkgs, ... }: {
  home.packages = with pkgs; [ easyeffects ];

  xdg.configFile."easyeffects" = {
    source = ../configs/easyeffects/.;
    recursive = true;
  };
}
