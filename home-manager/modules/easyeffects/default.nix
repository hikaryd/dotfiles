{ pkgs, ... }: {
  home.packages = with pkgs;
    [
      (writeShellScriptBin "easyeffects" ''
        export GSK_RENDERER=cairo
        exec ${easyeffects}/bin/easyeffects "$@"
      '')
    ];

  xdg.configFile."easyeffects" = {
    source = ./.;
    recursive = true;
  };
}
