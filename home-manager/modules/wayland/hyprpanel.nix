{ ... }: {
  programs.hyprpanel = {
    enable = false;
    hyprland.enable = true;
    overwrite.enable = true;

    theme = "tokyo_night";

    layout = {
      "bar.layouts" = {
        "0" = {
          left = [ "dashboard" "media" ];
          middle = [ "clock" ];
          right = [ "volume" "network" "bluetooth" "battery" "notifications" ];
        };
        "1" = {
          left = [ "dashboard" "media" ];
          middle = [ "clock" ];
          right = [ "volume" "network" "bluetooth" "battery" "notifications" ];
        };
      };
    };

    settings = {
      tear = true;
      scalingPriority = "hyprland";
      wallpaper.pywal = false;

      bar = {
        workspaces = {
          show_icons = true;
          show_numbered = false;
          numbered_active_indicator = "underline";
          workspaces = 10;
          monitorSpecific = false;
          # hideUnoccupied = false;
          workspaceMask = true;
          reverse_scroll = false;
          ignored = "-\\\\d+";
          showApplicationIcons = true;
          showWsIcons = true;
        };
      };

      menus = {
        transition = "crossfade";

        dashboard = {
          controls.enabled = false;
          directories.enabled = false;
          powermenu.avatar.name = "hikary";
          shortcuts.enabled = true;
        };

        clock = {
          time = {
            military = true;
            hideSeconds = true;
          };
          weather = {
            location = "Moscow";
            key = "d6f93bf44c7ae06b56225112ee80901d";
            unit = "metric";
          };
        };
      };

      theme = {
        matugen = false;
        bar = {
          floating = true;
          transparent = true;
          scaling = 70;

          buttons = {
            radius = "1.5em";
            style = "split";

            modules = {
              cpu.enableBorder = true;
              cpuTemp.enableBorder = true;
              kbLayout.enableBorder = true;
              netstat.enableBorder = true;
              ram.enableBorder = true;
            };
          };

          menus = {
            menu = {
              battery.scaling = 70;
              bluetooth.scaling = 70;
              clock.scaling = 70;
              dashboard = {
                confirmation_scaling = 70;
                scaling = 70;
              };
              media.scaling = 70;
              network.scaling = 70;
              notifications.scaling = 70;
              power.scaling = 70;
              volume.scaling = 70;
            };
            popover.scaling = 70;
          };
        };

        notification.scaling = 70;
        osd.scaling = 70;
        tooltip.scaling = 70;
      };
    };
  };
}
