{ ... }: {
  xdg.configFile."skhd" = {
    source = ../configs/skhd;
    recursive = true;
  };
}
