

{ pkgs, inputs, ... }: {
  home.packages = [ inputs.ashell.defaultPackage.${pkgs.system} ];

  xdg.configFile."ashell" = {
    source = ../configs/ashell/.;
    recursive = true;
  };
}
