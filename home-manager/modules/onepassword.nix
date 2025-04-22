{ pkgs, ... }:
let opPkg = pkgs._1password-gui;
in {
  home.packages = [ opPkg ];

  programs.bash.bashrcExtra = ''
    if command -v op &>/dev/null 2>&1; then
      eval "$(op agent --env)"
    fi
  '';

  xdg.configFile."zen-browser/native-messaging-hosts" = {
    source = "${opPkg}/share/native-messaging-hosts";
    force = true;
  };
}

