{...}: {
  programs.git = {
    enable = true;
  };

  xdg.configFile = {
    "git/gitk" = {
      source = ./gitk;
    };
    "git/hooks" = {
      source = ./hooks;
      recursive = true;
    };
  };
}
