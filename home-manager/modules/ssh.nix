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
in {
  programs.ssh = {
    enable = true;
    package = pkgs.openssh_hpn;
    forwardAgent = true;
    extraConfig = ''
      AddKeysToAgent yes
      ServerAliveInterval 5
      ServerAliveCountMax 10
    '';
    inherit (sshConfig) matchBlocks;
  };
  home.packages = with pkgs; [ mosh ];

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
