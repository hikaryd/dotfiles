{ pkgs, ... }: {
  home.file.".config/kanshi/config".text = ''
    profile laptop {
      output "eDP-1" {
        mode "2560x1600@120";
        position -2560,0;
        scale 1.6;
      }
      output "HDMI-A-1" {
        disable;
      }
    }

    profile HDMI {
      output "HDMI-A-1" {
        mode "2560x1440@120";
        position 0,0;
        scale 1.25;
      }
      output "eDP-1" {
        disable;
      }
    }
  '';

  home.packages = with pkgs; [ kanshi ];
}
