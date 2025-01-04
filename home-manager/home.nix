{ ... }: {
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
  };
}
