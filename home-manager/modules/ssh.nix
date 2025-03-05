{ pkgs, config, ... }:
let
  sshConfigPath = "${config.home.homeDirectory}/dotfiles/secrets/ssh.nix";
  sshConfigContent = builtins.tryEval (builtins.readFile sshConfigPath);
  sshConfig = if sshConfigContent.success then
    import (builtins.toFile "ssh.nix" sshConfigContent.value)
  else
    throw ''
      SSH config file not found at ${sshConfigPath}
      Please decrypt the GPG file first using:
      gpg -d secrets/ssh.nix.gpg > secrets/ssh.nix

      Or create this file by secrets/ssh.nix.example
    '';
  onePassPath = "~/.1password/agent.sock";
in {
  programs.ssh = {
    enable = true;
    package = pkgs.openssh_hpn;
    forwardAgent = true;
    extraConfig = ''
      AddKeysToAgent yes
      ServerAliveInterval 5
      ServerAliveCountMax 10
      IdentityAgent ${onePassPath}
    '';
    inherit (sshConfig) matchBlocks;
  };
  home.packages = with pkgs; [ mosh ];
}
