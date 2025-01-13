{ ... }: {
  wayland.windowManager.hyprland = {
    enable = true;
    systemd.enable = true;
    xwayland.enable = true;

    settings = {
      monitor = [
        "eDP-1, 2560x1600@120, -2560x0, 1.25"
        "desc:AOC Q27G2G3R3B RTEMAHA004734, 2560x1440@120, 0x0, 1.25"
      ];

      workspace = [
        "1, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, default:true"
        "2, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, default:true"
        "3, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, default:true"
        "4, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, default:true"
        "5, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, default:true"
        "6, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, default:true"
        "7, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, default:true"
        "8, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, default:true"
        "9, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, default:true"
        "10, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, default:true"
        "11, monitor:eDP-1, default:true"
        "special:telegram, on-created-empty:telegram-desktop"
        "special:audio, on-created-empty:audio"
        "special:music, on-created-empty:youtube-music"
        "special:vpn, on-created-empty:nekoray"
        "special:noi, on-created-empty:zen"
        "special:ai_ide"
      ];

      general = {
        gaps_in = 5;
        gaps_out = 14;
        border_size = 1;
        layout = "dwindle";
      };

      dwindle = {
        pseudotile = true;
        preserve_split = true;
        special_scale_factor = 0.9;
        smart_split = false;
      };

      master = {
        smart_resizing = true;
        new_on_active = true;
        drop_at_cursor = true;
        special_scale_factor = 0.9;
      };

      misc = {
        vfr = true;
        vrr = 0;
        animate_manual_resizes = true;
        animate_mouse_windowdragging = false;
        enable_swallow = false;
        swallow_regex =
          "(foot|kitty|allacritty|Alacritty|wzterm|com.mitchellh.ghostty)";
        disable_splash_rendering = true;
        disable_hyprland_logo = true;
        force_default_wallpaper = 0;
        new_window_takes_over_fullscreen = 2;
        allow_session_lock_restore = true;
        initial_workspace_tracking = false;
        key_press_enables_dpms = true;
        mouse_move_enables_dpms = true;
        focus_on_activate = false;
      };

      gestures = {
        workspace_swipe = true;
        workspace_swipe_distance = 700;
        workspace_swipe_fingers = 3;
        workspace_swipe_cancel_ratio = 0.2;
        workspace_swipe_min_speed_to_force = 5;
        workspace_swipe_direction_lock = true;
        workspace_swipe_direction_lock_threshold = 10;
        workspace_swipe_create_new = true;
      };

      xwayland = { force_zero_scaling = true; };

      windowrulev2 = [
        "workspace 1 silent, class:^(zen-beta)$"
        "workspace 1 silent, class:^(Google chrome)$"
        "workspace 11, class:^(zen-beta)$"
        "workspace 2 silent, class:^(dev)$"
        "workspace 3 silent, class:^(other)$"
        "workspace 4 silent, class:^(vesktop)$"
        "workspace 4 silent, class:^(discord)$"
        "workspace 7 silent, class:^(DBeaver)$"

        "immediate, class:^(mpv)$"
        "float,title:^(Google Chrome)(.*)$"
        "float,title:^(Untitled - Google Chrome)(.*)$"

        "workspace special:telegram, class:^(org.telegram.desktop)$"
        "workspace special:audio, class:^(pulsemixer)$"
        "workspace special:audio, class:^(com.github.wwmm.easyeffects)$"
        "workspace special:music, class:^(YouTube Music)$"
        "workspace special:vpn, class:^(nekoray)$"
        "workspace special:noi, class:^(zen-beta)$"
        "workspace special:ai_ide, class:^(codium)$"
        "workspace special:ai_ide, class:^(cursor-url-handler)$"

        "float, class:^(org.telegram.desktop)$"
        "float, class:^(com.github.wwmm.easyeffects)$"
        "float, class:^(pulsemixer)$"
        "float, class:^(nekoray)$"
        "float, class:^(pavucontrol)$"
        "float, class:^(blueman-manager)$"
        "float, class:^(nm-applet)$"
        "float, class:^(nm-connection-editor)$"
        "float, class:^(org.kde.polkit-kde-authentication-agent-1)$"
        "float, class:^(bluetooth-manager)$"
        "float, class:^(iwgtk)$"

        "opacity 0.80 0.80, class:^(org.freedesktop.impl.portal.desktop.gtk)$"
        "opacity 0.80 0.80, class:^(org.freedesktop.impl.portal.desktop.hyprland)$"
        "opacity 0.9 0.9,class:^(neovide)$"

        "float, class:^(YouTube Music)$"
        "size 800 1000, class:^(YouTube Music)$"
        "center, class:^(YouTube Music)$"

        "float, class:^(org.telegram.desktop)$"
        "size 700 1000, class:^(org.telegram.desktop)$"
        "center, class:^(org.telegram.desktop)$"

        "float, class:^(com.github.wwmm.easyeffects)$"
        "size 780 460, class:^(com.github.wwmm.easyeffects)$"
        "move 160 120, class:^(com.github.wwmm.easyeffects)$"

        "float, class:^(pulsemixer)$"
        "size 970 460, class:^(pulsemixer)$"
        "move 60 652, class:^(pulsemixer)$"

        "float, class:^(nekoray)$"
        "size 480 1000, class:^(nekoray)$"
        "move 200 80, class:^(nekoray)$"

        "opacity 0.9 0.9,class:^(zen-beta)$"
      ];

      input = {
        kb_layout = "us, ru";
        kb_options = "grp:caps_toggle";
        follow_mouse = 1;
        repeat_rate = 60;
        repeat_delay = 260;
        touchpad = {
          natural_scroll = true;
          disable_while_typing = true;
          drag_lock = false;
          tap-and-drag = false;
        };
        sensitivity = 0;
        force_no_accel = false;
      };

      env = [
        "TERM,ghostty"
        "WLR_DRM_NO_ATOMIC,1"
        "XDG_CURRENT_DESKTOP,Hyprland"
        "XDG_SESSION_TYPE,wayland"
        "XDG_SESSION_DESKTOP,Hyprland"
        "XDG_DATA_DIRS,$HOME/.nix-profile/share:/usr/local/share:/usr/share"
        "PATH,$HOME/.nix-profile/bin:$PATH"

        "QT_QPA_PLATFORM,wayland;xcb"
        "QT_WAYLAND_DISABLE_WINDOWDECORATION,1"
        "QT_AUTO_SCREEN_SCALE_FACTOR,1"
        "QT_QPA_PLATFORMTHEME,qt5ct"
        "QT_SCALE_FACTOR_ROUNDING_POLICY,RoundPreferFloor"
        "QT_WAYLAND_DISABLED_INTERFACES,wp_fractional_scale_manager_v1"

        "MOZ_ENABLE_WAYLAND,1"
        "GDK_SCALE,1"
        "SDL_VIDEODRIVER,wayland"
        "ELECTRON_ENABLE_WAYLAND,1"
        "ELECTRON_OZONE_PLATFORM_HINT,wayland"
        "WINIT_UNIX_BACKEND,wayland"

        "XCURSOR_THEME,Bibata-Modern-Ice"
        "XCURSOR_SIZE,20"
        "HYPRCURSOR_THEME,Bibata-Modern-Ice"
        "HYPRCURSOR_SIZE,20"
      ];

      animations = {
        enabled = true;
        bezier = [ "linear, 0,0,1,1" "swirl, 0.04, 1, 0.2, 1.2" ];
        animation = [
          "windows, 1, 4, swirl, popin 0%"
          "windowsOut, 1, 3, linear, popin 0%"
          "fade, 1, 2, linear"
          "workspaces, 1, 1, linear"
          "specialWorkspace, 1, 5, swirl, slidefadevert -50%"
        ];
      };

      decoration = {
        rounding = 12;
        shadow = {
          enabled = false;
          scale = 0.3;
        };
        blur = {
          enabled = true;
          xray = true;
          size = 4;
          passes = 5;
          ignore_opacity = true;
          brightness = 1.4;
          vibrancy = 0.3;
          vibrancy_darkness = 0.1;
          special = true;
          popups = true;
          noise = 5.0e-2;
          new_optimizations = true;
          contrast = 1;
        };
        active_opacity = 1;
        inactive_opacity = 1;
        fullscreen_opacity = 1;
        dim_around = 1;
        dim_inactive = true;
        dim_special = 0.3;
        dim_strength = 0.3;
      };

      "$mainMod" = "SUPER";
      bind = [
        "$mainMod, RETURN, exec, ghostty"
        "$mainMod SHIFT, D, exec, vesktop"
        "$mainMod SHIFT, B, exec, GDK_BACKEND=x11 dbeaver"

        "$mainMod, B, exec, hyprpanel -t bluetoothmenu"
        "$mainMod, N, exec, hyprpanel -t notificationsmenu"
        "$mainMod, S, exec, hyprpanel -t dashboardmenu"
        "$mainMod SHIFT, S, exec, ${../../../scripts/snapshot.sh}"
        "$mainMod SHIFT, R, exec, gpu-screen-recorder-gtk"
        "$mainMod, W, exec, hyprpanel -t networkmenu"

        "$mainMod, D, exec, anyrun"
        "$mainMod SHIFT CTRL, T, exec, ${../../../scripts/toggle-display.sh}"

        "$mainMod, Q, killactive"
        "$mainMod, delete, exit"
        "$mainMod, Space, togglefloating"
        "$mainMod SHIFT, F, fullscreen"
        "$mainMod CTRL, L, exec, systemctl suspend && hyprlock"

        "$mainMod, T, togglespecialworkspace, telegram"
        "$mainMod, P, togglespecialworkspace, music"
        "$mainMod, A, togglespecialworkspace, audio"
        "$mainMod, V, togglespecialworkspace, vpn"
        "$mainMod, G, togglespecialworkspace, noi"
        "$mainMod SHIFT, W, togglespecialworkspace, ai_ide"

        "$mainMod, J, focuswindow, next"
        "$mainMod, K, focuswindow, previous"
        "$mainMod SHIFT, j, movewindow, d"
        "$mainMod SHIFT, k, movewindow, u"
        "$mainMod SHIFT, h, movewindow, l"
        "$mainMod SHIFT, l, movewindow, r"

        "$mainMod ALT, H, moveactive, -100 0"
        "$mainMod ALT, J, moveactive, 0 100"
        "$mainMod ALT, K, moveactive, 0 -100"
        "$mainMod ALT, L, moveactive, 100 0"

        "$mainMod ALT CTRL, H, movewindow, l"
        "$mainMod ALT CTRL, J, movewindow, d"
        "$mainMod ALT CTRL, K, movewindow, u"
        "$mainMod ALT CTRL, L, movewindow, r"

        "$mainMod, Period, focusmonitor, +1"
        "$mainMod, Comma, focusmonitor, -1"
        "$mainMod SHIFT, Period, movewindow, mon:+1"
        "$mainMod SHIFT, Comma, movewindow, mon:-1"

        "$mainMod SHIFT ALT, k, exec, hyprctl dispatch moveintogroup l"
        "$mainMod SHIFT ALT, j, exec, hyprctl dispatch moveintogroup r"
        "$mainMod CTRL, p, pin, active"

        "$mainMod, 1, workspace, 1"
        "$mainMod, 2, workspace, 2"
        "$mainMod, 3, workspace, 3"
        "$mainMod, 4, workspace, 4"
        "$mainMod, 5, workspace, 5"
        "$mainMod, 6, workspace, 6"
        "$mainMod, 7, workspace, 7"
        "$mainMod, 8, workspace, 8"
        "$mainMod, 9, workspace, 9"
        "$mainMod, 0, workspace, 10"
        "$mainMod CTRL SHIFT, L, workspace, 11"

        "$mainMod SHIFT, 1, movetoworkspacesilent, 1"
        "$mainMod SHIFT, 2, movetoworkspacesilent, 2"
        "$mainMod SHIFT, 3, movetoworkspacesilent, 3"
        "$mainMod SHIFT, 4, movetoworkspacesilent, 4"
        "$mainMod SHIFT, 5, movetoworkspacesilent, 5"
        "$mainMod SHIFT, 6, movetoworkspacesilent, 6"
        "$mainMod SHIFT, 7, movetoworkspacesilent, 7"
        "$mainMod SHIFT, 8, movetoworkspacesilent, 8"
        "$mainMod SHIFT, 9, movetoworkspacesilent, 9"
        "$mainMod SHIFT, 0, movetoworkspacesilent, 10"

        "$mainMod CTRL, right, workspace, r+1"
        "$mainMod CTRL, left, workspace, r-1"
        "$mainMod CTRL, down, workspace, empty"

        "$mainMod, h, movefocus, l"
        "$mainMod, l, movefocus, r"
        "$mainMod, k, movefocus, u"
        "$mainMod, j, movefocus, d"

        "$mainMod SHIFT, h, movewindow, l"
        "$mainMod SHIFT, l, movewindow, r"
        "$mainMod SHIFT, k, movewindow, u"
        "$mainMod SHIFT, j, movewindow, d"
      ];

      bindm = [
        "$mainMod, mouse:272, movewindow"
        "$mainMod, mouse:273, resizewindow"
      ];

      exec-once = [
        "nekoray"
        "hyprpaper"
        "dbus-update-activation-environment --systemd WAYLAND_DISPLAY XDG_CURRENT_DESKTOP"
        "${../../../scripts/xdg-portal.sh}"
        "${../../../scripts/check-airpods.sh}"
        "easyeffects"
        "blueman-applet"
        "hypridle"
        "telegram-desktop"
        "wl-paste -t text -w xclip -selection clipboard"
        "kitty --class pulsemixer -- pulsemixer"
        "sleep 4 && zen"
      ];
    };
  };

}

