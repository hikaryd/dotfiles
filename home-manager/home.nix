{ user, ... }: {
  home = {
    enableNixpkgsReleaseCheck = false;
    username = user;
    homeDirectory = "/Users/${user}";
    stateVersion = "24.11";
  };

  imports = [ ./theme.nix ./modules ./packages.nix ];

  programs.home-manager.enable = true;
  services.mako.enable = false;
}
