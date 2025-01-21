{ config, pkgs, ... }:
let
  pointer = config.home.pointerCursor;
  makeCommand = command: { command = [ command ]; };
in {
  programs.niri = {
    enable = true;
    package = config.lib.nixGL.wrap pkgs.niri;
    settings = {
      environment = {
        CLUTTER_BACKEND = "wayland";
        DISPLAY = ":1";
        GDK_BACKEND = "wayland,x11";
        MOZ_ENABLE_WAYLAND = "1";
        NIXOS_OZONE_WL = "1";
        QT_QPA_PLATFORM = "wayland;xcb";
        QT_WAYLAND_DISABLE_WINDOWDECORATION = "1";
        SDL_VIDEODRIVER = "wayland";
        XDG_DATA_DIRS = "$HOME/.nix-profile/share:/usr/local/share:/usr/share";
        PATH = "$HOME/.nix-profile/bin:$PATH";
      };
      spawn-at-startup = [
        (makeCommand
          "${pkgs.polkit_gnome}/libexec/polkit-gnome-authentication-agent-1")
        (makeCommand "hyprpanel")
        (makeCommand "wl-paste --type image --watch cliphist store")
        (makeCommand "wl-paste --type text --watch cliphist store")
        (makeCommand
          "dbus-update-activation-environment --systemd WAYLAND_DISPLAY XDG_CURRENT_DESKTOP")

        (makeCommand "telegram-desktop")
        (makeCommand "nekoray")
        (makeCommand "easyeffects")
        (makeCommand "sleep 4 && zen")
      ];
      input = {
        keyboard = {
          xkb.layout = "us, ru";
          xkb.options = "grp:caps_toggle";
          repeat-rate = 60;
          repeat-delay = 260;
        };
        touchpad = {
          click-method = "button-areas";
          dwt = true;
          dwtp = true;
          natural-scroll = true;
          scroll-method = "two-finger";
          tap = true;
          tap-button-map = "left-right-middle";
        };
        focus-follows-mouse.enable = true;
        warp-mouse-to-focus = true;
        workspace-auto-back-and-forth = true;
      };
      outputs = {
        "eDP-1" = {
          scale = 1.25;
          mode = {
            width = 2560;
            height = 1600;
            refresh = 120.006;
          };
          position = {
            x = -2560;
            y = 0;
          };
        };
        "HDMI-A-1" = {
          mode = {
            width = 2560;
            height = 1440;
            refresh = 119.998;
          };
          scale = 1.25;
          position = {
            x = 0;
            y = 0;
          };
        };
      };
      cursor = {
        size = 20;
        theme = "${pointer.name}";
        hide-after-inactive-ms = 1000;
        hide-when-typing = true;
      };
      layout = {
        focus-ring.enable = false;
        border = {
          enable = true;
          width = 1;
          active.color = "#16aff1";
          inactive.color = "#245b89";
        };

        preset-column-widths = [
          { proportion = 1.0 / 3.0; }
          { proportion = 1.0 / 2.0; }
          { proportion = 2.0 / 3.0; }
          { proportion = 1.0; }
        ];
        default-column-width = { proportion = 1.0 / 2.0; };

        gaps = 8;
        struts = {
          left = 0;
          right = 0;
          top = 0;
          bottom = 0;
        };
      };

      animations.shaders.window-resize = ''
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

      window-rules = [
        {
          geometry-corner-radius = let radius = 16.0;
          in {
            bottom-left = radius;
            bottom-right = radius;
            top-left = radius;
            top-right = radius;
          };
          clip-to-geometry = true;
          draw-border-with-background = false;
        }
        {
          matches = [{ app-id = "org.telegram.desktop"; }];
          block-out-from = "screencast";
        }
        {
          matches = [{ app-id = "app.drey.PaperPlane"; }];
          block-out-from = "screencast";
        }
        { matches = [{ app-id = "com.rafaelmardojai.Blanket"; }]; }
        { matches = [{ app-id = "org.nickvision.cavalier"; }]; }
      ];

      prefer-no-csd = true;
      hotkey-overlay.skip-at-startup = true;
    };
  };
}

