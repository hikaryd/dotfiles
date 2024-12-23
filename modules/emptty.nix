{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.xserver.displayManager.emptty;
in
{
  options = {
    services.xserver.displayManager.emptty = {
      enable = mkEnableOption "emptty display manager";

      config = mkOption {
        type = types.attrs;
        default = {};
        description = "Configuration for emptty";
      };
    };
  };

  config = mkIf cfg.enable {
    services.xserver.displayManager = {
      lightdm.enable = mkForce false;
      gdm.enable = mkForce false;
      sddm.enable = mkForce false;
      job.execCmd = mkForce "${pkgs.emptty}/bin/emptty";
    };

    environment.systemPackages = [ pkgs.emptty ];

    security.pam.services.emptty = {
      allowNullPassword = true;
      startSession = true;
    };

    systemd.services.emptty = {
      description = "Emptty Display Manager";
      after = [ "systemd-user-sessions.service" "plymouth-quit-wait.service" "getty@tty1.service" ];
      conflicts = [ "getty@tty1.service" ];
      wantedBy = [ "multi-user.target" ];

      serviceConfig = {
        Type = "simple";
        ExecStart = "${pkgs.emptty}/bin/emptty";
        StandardInput = "tty";
        StandardOutput = "tty";
        StandardError = "journal";
        TTYPath = "/dev/tty1";
        TTYReset = "yes";
        TTYVHangup = "yes";
        Restart = "always";
      };
    };
  };
}
