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

      inactiveOpacity = 0.7;

      monitorInternal = "eDP-1";
      monitorExternal = "HDMI-A-1";
      makeCommand = command: { command = [ command ]; };

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

      animations = {
        enable = true;

        shaders.window-resize = ''
          vec4 resize_color(vec3 coords_curr_geo, vec3 size_curr_geo) {
            vec3 coords_next_geo = niri_curr_geo_to_next_geo * coords_curr_geo;

            vec3 coords_stretch = niri_geo_to_tex_next * coords_curr_geo;
            vec3 coords_crop = niri_geo_to_tex_next * coords_next_geo;

            // We can crop if the current window size is smaller than the next window
            // size. One way to tell is by comparing to 1.0 the X and Y scaling
            // coefficients in the current-to-next transformation matrix.
            bool can_crop_by_x = niri_curr_geo_to_next_geo[0][0] <= 1.0;
            bool can_crop_by_y = niri_curr_geo_to_next_geo[1][1] <= 1.0;

            vec3 coords = coords_stretch;
            if (can_crop_by_x)
                coords.x = coords_crop.x;
            if (can_crop_by_y)
                coords.y = coords_crop.y;

            vec4 color = texture2D(niri_tex_next, coords.st);

            // However, when we crop, we also want to crop out anything outside the
            // current geometry. This is because the area of the shader is unspecified
            // and usually bigger than the current geometry, so if we don't fill pixels
            // outside with transparency, the texture will leak out.
            //
            // When stretching, this is not an issue because the area outside will
            // correspond to client-side decoration shadows, which are already supposed
            // to be outside.
            if (can_crop_by_x && (coords_curr_geo.x < 0.0 || 1.0 < coords_curr_geo.x))
                color = vec4(0.0);
            if (can_crop_by_y && (coords_curr_geo.y < 0.0 || 1.0 < coords_curr_geo.y))
                color = vec4(0.0);

            return color;
          }
        '';
      };

      prefer-no-csd = true;
      hotkey-overlay.skip-at-startup = true;

      layout = {
        focus-ring.enable = false;
        border = {
          enable = false;
          width = 1;
          active.color = focusRingColor;
          inactive.color = inactiveBorderColor;
        };
        shadow = { enable = true; };
        preset-column-widths = [
          { proportion = 0.25; }
          { proportion = 0.5; }
          { proportion = 0.75; }
          { proportion = 1.0; }
        ];
        default-column-width = { proportion = 0.5; };
        gaps = 6;
        struts = {
          left = 0;
          right = 0;
          top = 0;
          bottom = 0;
        };
        tab-indicator = {
          hide-when-single-tab = true;
          place-within-column = true;
          position = "left";
          corner-radius = 20.0;
          gap = -12.0;
          gaps-between-tabs = 10.0;
          width = 4.0;
          length.total-proportion = 0.1;
        };
      };

      input = {
        focus-follows-mouse.enable = true;
        warp-mouse-to-focus = true;
        workspace-auto-back-and-forth = true;
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
          click-method = "button-areas";
          dwt = true;
          dwtp = true;
          natural-scroll = true;
          scroll-method = "two-finger";
          tap = true;
          tap-button-map = "left-right-middle";
          middle-emulation = true;
          accel-profile = "adaptive";
        };
      };

      cursor = {
        theme = "Bibata-Modern-Ice";
        size = 20;
        hide-when-typing = false;
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
        QT_AUTO_SCREEN_SCALE_FACTOR = "1";
        QT_QPA_PLATFORMTHEME = "qt5ct";
        QT_SCALE_FACTOR_ROUNDING_POLICY = "RoundPreferFloor";
        QT_WAYLAND_DISABLED_INTERFACES = "wp_fractional_scale_manager_v1";
        CLUTTER_BACKEND = "wayland";
        DISPLAY = null;
        GDK_BACKEND = "wayland,x11";
        MOZ_ENABLE_WAYLAND = "1";
        NIXOS_OZONE_WL = "1";
        QT_QPA_PLATFORM = "wayland;xcb";
        QT_WAYLAND_DISABLE_WINDOWDECORATION = "1";
        SDL_VIDEODRIVER = "wayland";
      };

      spawn-at-startup = [
        (makeCommand "ashell")
        (makeCommand "${pkgs.kanata}/bin/kanata")
        (makeCommand "slack")
        (makeCommand "jamedsp")
        (makeCommand "1password")
        (makeCommand "${pkgs.hyprpaper}/bin/hyprpaper")
        (makeCommand "dunst")
        (makeCommand "${pkgs.tidal-hifi}/bin/tidal-hifi")
        (makeCommand
          "${pkgs.easyeffects}/bin/easyeffects --gapplication-service")
        (makeCommand "google-chrome-stable")
        (makeCommand "ayugram-desktop")
        (makeCommand "wl-paste --type image --watch cliphist store")
        (makeCommand "wl-paste --type text --watch cliphist store")
        {
          command = [
            "${pkgs.dbus}/bin/dbus-update-activation-environment"
            "--systemd"
            "DISPLAY"
            "WAYLAND_DISPLAY"
            "SWAYSOCK"
            "XDG_CURRENT_DESKTOP"
            "XDG_SESSION_TYPE"
            "NIXOS_OZONE_WL"
            "XCURSOR_THEME"
            "XCURSOR_SIZE"
            "XDG_DATA_DIRS"
          ];
        }
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
          matches = [{ app-id = "^(com.mitchellh.ghostty|Slack)$"; }];
          open-on-workspace = "1";
        }
        {
          matches = [{
            app-id =
              "^(zoom|zoom-us|Zoom Workplace|com.github.wwmm.easyeffects|tidal-hifi)$";
          }];
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
        { matches = [{ app-id = "^(neovide)$"; }]; }
        { matches = [{ app-id = "^(DBeaver)$"; }]; }
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
        { matches = [{ app-id = ""; }]; }
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
          action = { move-window-to-workspace = [ "2" ]; };
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
