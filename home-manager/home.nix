{ config, lib, pkgs, ... }: {
  home = {
    username = "hikary";
    homeDirectory = "/home/hikary";
    stateVersion = "24.11";

  };
  imports = [ ./modules ./theme.nix ./packages.nix ];
  programs.home-manager.enable = true;

  home.sessionPath = [ "$HOME/.nix-profile/bin" ];
  home.sessionVariables = {
    EDITOR = "nvim";
    VISUAL = "nvim";
    NVIM_APPNAME = "nvim";
    SSH_AUTH_SOCK = "$XDG_RUNTIME_DIR/ssh-agent.socket";
    GI_TYPELIB_PATH =
      "${pkgs.gtk3}/lib/girepository-1.0:${pkgs.pango}/lib/girepository-1.0";
    LD_LIBRARY_PATH = "${pkgs.glib}/lib:${pkgs.gtk3}/lib:$LD_LIBRARY_PATH";
  };
}
