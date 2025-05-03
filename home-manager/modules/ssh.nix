{ pkgs, ... }:
let
  agent = if !pkgs.stdenv.isDarwin then
    "~/.1password/agent.sock"
  else
    "~/Library/Group Containers/2BUA8C4S2C.com.1password/t/agent.sock";
in {
  programs.ssh = {
    enable = true;
    package = pkgs.openssh_hpn;
    forwardAgent = true;
    extraConfig = ''
      AddKeysToAgent yes
      ServerAliveInterval 5
      ServerAliveCountMax 10
      IdentityAgent "${agent}"
    '';
  };
  home.packages = with pkgs; [ mosh ];
}
