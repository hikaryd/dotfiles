{ ... }: {
  home.file.".config/zed/" = {
    source = ../configs/zed;
    recursive = true;
  };
}
