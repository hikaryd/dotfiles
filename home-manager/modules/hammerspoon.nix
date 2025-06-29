{ ... }: {
  home.file.".hammerspoon" = {
    source = ../configs/hammerspoon;
    recursive = true;
  };
}
