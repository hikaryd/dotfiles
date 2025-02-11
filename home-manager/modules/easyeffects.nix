{ pkgs, config, ... }: {
  home.packages = with pkgs;
    [
      (writeShellScriptBin "easyeffects" ''
        export GSK_RENDERER=cairo
        exec ${(config.lib.nixGL.wrap easyeffects)}/bin/easyeffects "$@"
      '')
    ];

  xdg.configFile."easyeffects" = {
    source = ../configs/easyeffects/.;
    recursive = true;
  };
}
