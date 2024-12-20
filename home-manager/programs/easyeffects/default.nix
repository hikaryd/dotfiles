{pkgs, ...}: {
  home.packages = with pkgs; [
    easyeffects
  ];

  xdg.configFile."easyeffects" = {
    source = ./.;
    recursive = true;
  };
}
