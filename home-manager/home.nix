{ pkgs, inputs, system, lib, ... }: {
  home = {
    username = "hikary";
    homeDirectory = if system == "x86_64-linux" then "/home/hikary" else "/Users/hikary";
    stateVersion = "24.11";
  };

  imports = [
    ./modules
    ./theme.nix
    ./packages.nix
  ];

  programs.home-manager.enable = true;

  nixGL = {
    packages = inputs.nixgl.packages.${pkgs.system};
    defaultWrapper = "mesaPrime";
  };

  programs.bash = {
    enable = true;
    bashrcExtra = ''
      export LANG="en_US.UTF-8"
      export LC_ALL="en_US.UTF-8"
    '';
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
