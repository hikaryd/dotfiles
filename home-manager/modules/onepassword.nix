{ pkgs, ... }:
let opPkg = pkgs._1password-gui;
in {
  home.packages = [ opPkg ];

  xdg.configFile."zen-browser/native-messaging-hosts" = {
    source = "${opPkg}/share/native-messaging-hosts";
    force = true;
  };
}

