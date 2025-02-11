{ pkgs, ... }: {
  home.packages = with pkgs; [ lazygit ];

  xdg.configFile."lazygit/config.yml" = {
    source = ../configs/lazygit/config.yml;
  };
}
