{ pkgs, ... }: {
  programs.ssh = {
    enable = true;
    package = pkgs.openssh_hpn;

    forwardAgent = true;

    extraConfig = ''
      AddKeysToAgent yes
    '';

    matchBlocks = {
      "nexus" = {
        hostname = "109.120.137.85";
        user = "hikary";
        identityFile = "~/.ssh/nexus";
        port = 22132;
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
        hostname = "38.180.199.244";
        user = "www";
        identityFile = "~/.ssh/hikary";
      };

      "vpn_timur" = {
        hostname = "5.61.60.229";
        user = "root";
        identityFile = "~/.ssh/hikary";
      };

      "warden" = {
        hostname = "176.57.220.127";
        user = "root";
        identityFile = "~/.ssh/hikary";
      };

      "kvant-mgmt" = {
        hostname = "84.201.184.187";
        user = "egor";
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
