{ pkgs, ... }: {
  programs.ssh = {
    enable = true;
    package = pkgs.openssh_hpn;
    forwardAgent = true;
    extraConfig = ''
      AddKeysToAgent yes
      ServerAliveInterval 5
      ServerAliveCountMax 10
      Host *
            IdentityAgent "~/Library/Group Containers/2BUA8C4S2C.com.1password/t/agent.sock"
    '';
  };
}
