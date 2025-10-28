{ pkgs, ... }: {
  programs.ssh = {
    enable = true;
    enableDefaultConfig = false;
    package = pkgs.openssh_hpn;
    matchBlocks = { "*" = { forwardAgent = true; }; };
    extraConfig = ''
      AddKeysToAgent yes
      ServerAliveInterval 5
      ServerAliveCountMax 10
      IdentityAgent "~/.1password/agent.sock"
    '';
  };
}

