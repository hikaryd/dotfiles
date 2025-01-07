{ pkgs, ... }:
let hosts = import ./ssh-hosts.nix;
in {
  programs.ssh = {
    enable = true;
    package = pkgs.openssh_hpn;
    forwardAgent = true;
    extraConfig = ''
      AddKeysToAgent yes
    '';

    matchBlocks = {
      "nexus" = {
        inherit (hosts.nexus) hostname user port;
        identityFile = "~/.ssh/nexus";
      };

      "github.com" = {
        hostname = "github.com";
        identityFile = "~/.ssh/hikary";
        extraOptions = { AddKeysToAgent = "yes"; };
      };

      "gitlab.com" = {
        hostname = "gitlab.com";
        identityFile = "~/.ssh/hikary";
        extraOptions = { AddKeysToAgent = "yes"; };
      };

      "gitlab.kvantpro.com" = {
        hostname = "gitlab.kvantpro.com";
        identityFile = "~/.ssh/hikary";
        extraOptions = { AddKeysToAgent = "yes"; };
      };

      "misc" = {
        inherit (hosts.misc) hostname user;
        identityFile = "~/.ssh/hikary";
      };

      "vpn_timur" = {
        inherit (hosts.vpn_timur) hostname user;
        identityFile = "~/.ssh/hikary";
      };

      "warden" = {
        inherit (hosts.warden) hostname user;
        identityFile = "~/.ssh/hikary";
      };

      "kvant-mgmt" = {
        inherit (hosts.kvant-mgmt) hostname user;
        identityFile = "~/.ssh/hikary";
        forwardAgent = true;
        extraOptions = { AddKeysToAgent = "yes"; };
      };
    };
  };

  systemd.user.services.ssh-agent = {
    Unit = { Description = "SSH key agent"; };

    Service = {
      Type = "simple";
      Environment = "SSH_AUTH_SOCK=%t/ssh-agent.socket";
      ExecStart = "${pkgs.openssh_hpn}/bin/ssh-agent -D -a $SSH_AUTH_SOCK";
    };

    Install = { WantedBy = [ "default.target" ]; };
  };
}
