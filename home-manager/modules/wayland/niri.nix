{ pkgs, inputs, config, ... }: {

  imports = [ inputs.niri.homeModules.niri ];

  programs.niri = {
    enable = true;
    package = config.lib.nixGL.wrap pkgs.niri-unstable;

    settings = let
      mod = "Super";
      shift = "Shift";
      ctrl = "Control";
      alt = "Alt";

      term = "ghostty";
      launcher = "anyrun";

      inactiveBorderColor = "rgb(80 80 80)";
      focusRingColor = "rgb(66 153 225)";
      inactiveFocusRingColor = inactiveBorderColor;
      shadowColor = "#0000004C";

      inactiveOpacity = 0.1;

      monitorInternal = "eDP-1";
      monitorExternal = "HDMI-A-1";

    in {
      outputs = {
        "${monitorInternal}" = {
          enable = true;
          mode = {
            width = 2560;
            height = 1600;
            refresh = null;
          };
          position = {
            x = -2560;
            y = 0;
          };
          scale = 1.25;
        };
        "${monitorExternal}" = {
          enable = true;
          mode = {
            width = 2560;
            height = 1440;
            refresh = null;
          };
          position = {
            x = 0;
            y = 0;
          };
          scale = 1.0;
        };
      };

      animations = { enable = true; };

      layout = {
        gaps = 5;
        tab-indicator = {
          enable = true;
          width = 1;
          active.color = focusRingColor;
          inactive.color = inactiveBorderColor;
        };
        focus-ring = {
          enable = false;
          width = 2;
          active.color = focusRingColor;
          inactive.color = inactiveFocusRingColor;
        };
        shadow = {
          enable = true;
          softness = 10;
          spread = 5;
          offset = {
            x = 0;
            y = 5;
          };
          color = shadowColor;
        };
      };

      input = {
        focus-follows-mouse = { enable = true; };
        keyboard = {
          repeat-delay = 260;
          repeat-rate = 60;
          xkb = {
            layout = "us,ru";
            options = "grp:caps_toggle";
          };
        };
        mouse = { natural-scroll = false; };
        touchpad = {
          natural-scroll = false;
          dwt = true;
          tap = true;
        };
      };

      cursor = {
        theme = "Bibata-Modern-Ice";
        size = 20;
        hide-when-typing = true;
      };

      gestures = {
        dnd-edge-view-scroll = {
          delay-ms = 100;
          max-speed = 1500.0;
          trigger-width = 30.0;
        };
      };

      workspaces = {
        "1" = { open-on-output = "HDMI-A-1"; };
        "2" = { open-on-output = "eDP-1"; };
        "other" = { open-on-output = "HDMI-A-1"; };
      };
      environment = {
        XDG_DATA_DIRS = "$HOME/.nix-profile/share:/usr/local/share:/usr/share";
        DISPLAY = ":0";
      };

      spawn-at-startup = [
        { command = [ "xwayland-satellite" ]; }
        {
          command = [
            "dbus-update-activation-environment --systemd WAYLAND_DISPLAY XDG_CURRENT_DESKTOP"
          ];
        }
        { command = [ "google-chrome-stable" ]; }
        { command = [ "${pkgs.waybar}/bin/waybar" ]; }
        { command = [ "${pkgs.hyprpaper}/bin/hyprpaper" ]; }
        { command = [ "${pkgs.kanata}/bin/kanata" ]; }
        { command = [ "${pkgs.slack}/bin/slack" ]; }
        { command = [ "${pkgs.ghostty}/bin/ghostty" ]; }
        { command = [ "${pkgs.tidal-hifi}/bin/tidal-hifi" ]; }
        { command = [ "google-chrome-stable" ]; }
        { command = [ "ayugram-desktop" ]; }
      ];

      window-rules = [
        {
          matches = [ ];
          geometry-corner-radius = {
            top-left = 28.0;
            top-right = 28.0;
            bottom-left = 28.0;
            bottom-right = 28.0;
          };
          clip-to-geometry = true;
        }
        {
          matches = [{ is-focused = false; }];
          opacity = inactiveOpacity;
        }
        {
          matches = [{ app-id = "^(com.mitchellh.ghostty)$"; }];
          open-on-workspace = "1";
        }
        { matches = [{ app-id = "^(neovide)$"; }]; }
        { matches = [{ app-id = "^(DBeaver)$"; }]; }
        {
          matches = [{ app-id = "^(zoom|zoom-us|Zoom Workplace)$"; }];
          open-on-workspace = "2";
        }
        {
          matches = [{ app-id = "^(tidal-hifi)$"; }];
          open-on-workspace = "2";
        }
        {
          matches = [{
            app-id =
              "^(google-chrome|google-chrome-stable|com.ayugram.desktop)$";
          }];
          open-on-workspace = "other";
          opacity = 0.9;
        }
        {
          matches =
            [{ title = "^(Google Chrome.*|Untitled - Google Chrome)$"; }];
          open-floating = true;
        }
        {
          matches = [{ title = "^JamesDSP for Linux.*$"; }];
          open-floating = true;
          default-window-height = { fixed = 400; };
        }
        {
          matches =
            [{ app-id = "^(pulsemixer|nekoray|hiddify|pavucontrol)$"; }];
          open-floating = true;
        }
        {
          matches = [{
            app-id = "^(blueman-manager|nm-applet|nm-connection-editor)$";
          }];
          open-floating = true;
        }
        {
          matches = [{ app-id = "org.kde.polkit-kde-authentication-agent-1"; }];
          open-floating = true;
        }
        {
          matches = [{ app-id = "^(bluetooth-manager|iwgtk)$"; }];
          open-floating = true;
        }
        {
          matches = [{ app-id = "1Password"; }];
          open-floating = true;
          default-window-height = { fixed = 700; };
        }
        {
          matches = [{ app-id = "com.github.wwmm.easyeffects"; }];
          open-floating = true;
          default-window-height = { fixed = 400; };
        }
        {
          matches = [{ app-id = "xdg-desktop-portal-gtk"; }];
          open-floating = true;
          default-window-height = { fixed = 400; };
        }
        {
          matches = [{
            app-id =
              "^(org.freedesktop.impl.portal.desktop.gtk|org.freedesktop.impl.portal.desktop.hyprland)$";
          }];
          opacity = 0.8;
        }
      ];

      binds = {
        "${mod}+Return" = { action = { spawn = [ term ]; }; };
        "${mod}+A" = { action = { spawn = [ launcher ]; }; };

        "${mod}+Q" = { action = { close-window = [ ]; }; };
        "${mod}+${shift}+F" = { action = { toggle-window-floating = [ ]; }; };
        "${mod}+${shift}+Space" = { action = { fullscreen-window = [ ]; }; };

        "${mod}+H" = { action = { focus-column-or-monitor-left = [ ]; }; };
        "${mod}+L" = { action = { focus-column-or-monitor-right = [ ]; }; };

        "${mod}+${shift}+J" = {
          action = { move-window-to-workspace-down = [ ]; };
        };
        "${mod}+${shift}+K" = {
          action = { move-window-to-workspace-up = [ ]; };
        };
        "${mod}+${shift}+H" = {
          action = { consume-or-expel-window-left = [ ]; };
        };
        "${mod}+${shift}+L" = {
          action = { consume-or-expel-window-right = [ ]; };
        };

        "${mod}+${alt}+H" = { action = { set-column-width = "-100"; }; };
        "${mod}+${alt}+L" = { action = { set-column-width = "+100"; }; };

        "${mod}+K" = { action = { focus-workspace = [ "1" ]; }; };
        "${mod}+G" = { action = { focus-workspace = [ "2" ]; }; };
        "${mod}+J" = { action = { focus-workspace = [ "other" ]; }; };

        "${mod}+${shift}+G" = {
          action = { move-window-to-workspace = [ "1" ]; };
        };
        "${mod}+${shift}+B" = {
          action = { move-window-to-workspace = [ "2" ]; };
        };
        "${mod}+${shift}+E" = {
          action = { move-window-to-workspace = [ "other" ]; };
        };

        "${mod}+Period" = { action = { focus-monitor-next = [ ]; }; };
        "${mod}+Comma" = { action = { focus-monitor-previous = [ ]; }; };
        "${mod}+${shift}+Period" = {
          action = { move-window-to-monitor-next = [ ]; };
        };
        "${mod}+${shift}+Comma" = {
          action = { move-window-to-monitor-previous = [ ]; };
        };

        "${ctrl}+${shift}+S" = { action = { screenshot = { }; }; };
      };
    };

  };
}
