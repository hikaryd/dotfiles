{ pkgs, ... }: {
  programs.ssh = {
    enable = true;
    package = pkgs.openssh_hpn;
    forwardAgent = true;
    extraConfig = ''
      AddKeysToAgent yes
      ServerAliveInterval 5
      ServerAliveCountMax 10
      IdentityAgent "~/.1password/agent.sock"
    '';
  };
  home.packages = with pkgs; [ mosh ];
}
