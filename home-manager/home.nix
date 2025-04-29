{ pkgs, inputs, system, lib, user, ... }: {
  home = {
    enableNixpkgsReleaseCheck = false;
    username = user;
    homeDirectory =
      if system == "x86_64-linux" then "/home/${user}" else "/Users/${user}";
    stateVersion = "24.11";
  };

  imports = if system == "x86_64-linux" then [
    ./modules
    ./packages.nix
    ./theme.nix
  ] else [
    ./modules
    ./packages.nix
  ];

  programs.home-manager.enable = true;

  programs.bash = lib.mkIf (system == "x86_64-linux") {
    enable = true;
    bashrcExtra = ''
      export LANG="en_US.UTF-8"
      export LC_ALL="en_US.UTF-8"
    '';
  };

  nixGL = lib.mkIf (system == "x86_64-linux") {
    packages = inputs.nixgl.packages.${pkgs.system};
    defaultWrapper = "mesaPrime";
  };
  systemd = lib.mkIf (system == "x86_64-linux") {
    user.services = {
      kanata = {
        Unit = {
          Description = "Kanata Key Remapper";
          After = [ "graphical-session-pre.target" ];
          PartOf = [ "graphical-session.target" ];
        };
        Service = {
          Type = "simple";
          ExecStart = "${pkgs.kanata}/bin/kanata";
          Restart = "on-failure";
          RestartSec = 5;
        };
        Install = { WantedBy = [ "graphical-session.target" ]; };
      };

      polkitAgent = {
        Unit = {
          Description = "Polkit Authentication Agent";
          After = [ "graphical-session-pre.target" ];
          PartOf = [ "graphical-session.target" ];
        };
        Service = {
          Type = "simple";
          ExecStart = "/usr/lib/polkit-kde-authentication-agent-1";
          Restart = "on-failure";
          RestartSec = 5;
        };
        Install = { WantedBy = [ "graphical-session.target" ]; };
      };
    };
  };
}
