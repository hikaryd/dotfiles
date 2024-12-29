{ pkgs, inputs, ... }: {
  xdg.configFile."ghostty/config".source = ./ghostty/config;

  home.packages = [
    (pkgs.writeShellScriptBin "ghostty" ''
      ${pkgs.nixgl.nixGLMesa}/bin/nixGLMesa ${inputs.ghostty.packages.${pkgs.system}.default}/bin/ghostty "$@"
    '')
  ];
}
