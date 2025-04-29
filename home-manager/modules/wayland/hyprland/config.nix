{ config, pkgs, ... }: {
  wayland.windowManager.hyprland = {
    enable = true;
    package = config.lib.nixGL.wrap pkgs.hyprland;
    systemd.enable = true;
    systemd.variables = [ "--all" ];
    xwayland.enable = true;
    settings = {
      monitor = [
        "eDP-1, 2560x1600@120, auto, 1.25"
        "HDMI-A-1, 2560x1440@144, auto, 1,bitdepth, 10, cm, wide"
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
        vrr = 1;
        animate_manual_resizes = true;
        animate_mouse_windowdragging = false;
        enable_swallow = false;
        swallow_regex =
          "(foot|allacritty|Alacritty|wzterm|com.mitchellh.ghostty)";
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
      layerrule = [ "blur ,gtk-layer-shell" "ignorezero ,gtk-layer-shell" ];
      windowrule = [ "noanim, floating:1" ];

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

        "QT_QPA_PLATFORM,wayland;xcb"
        "QT_WAYLAND_DISABLE_WINDOWDECORATION,1"
        "QT_AUTO_SCREEN_SCALE_FACTOR,1"
        "QT_QPA_PLATFORMTHEME,qt5ct"
        "QT_SCALE_FACTOR_ROUNDING_POLICY,RoundPreferFloor"
        "QT_WAYLAND_DISABLED_INTERFACES,wp_fractional_scale_manager_v1"

        "WLR_DRM_NO_ATOMIC,1"
        "MOZ_ENABLE_WAYLAND,1"
        "GDK_SCALE,1"
        "SDL_VIDEODRIVER,wayland"
        "ELECTRON_ENABLE_WAYLAND,1"
        "ELECTRON_OZONE_PLATFORM_HINT,wayland"
        "WINIT_UNIX_BACKEND,wayland"
        "WLR_RENDERER,vulkan"
        "WLR_RENDERER_ALLOW_SOFTWARE,1"

        "HYPRCURSOR_SIZE,23"
        "HYPRCURSOR_THEME,phinger-cursors-light"

        "XCURSOR_THEME,phinger-cursors-light"
        "XCURSOR_SIZE,23"
      ];

      animations = {
        enabled = true;
        bezier = [ "soft, 0.25, 0.8, 0.3, 1" ];
        animation = [
          "global,       1, 8,   soft"
          "windowsIn,    1, 5,   soft, popin 80%"
          "windowsOut,   1, 4,   soft, popin 80%"
          "windowsMove,  1, 3,   linear"
          "workspaces,   1, 4,   soft, slidefade 12%"
          "fadeIn,       1, 3,   linear"
          "fadeOut,      1, 3,   linear"
        ];
      };

      decoration = {
        rounding = 28;
        shadow = {
          enabled = true;
          range = 20;
          render_power = 4;
        };
        blur = {
          enabled = true;
          size = 2;
          passes = 3;
          new_optimizations = true;
          ignore_opacity = true;
          special = true;
          popups = true;
        };
        active_opacity = 1;
        inactive_opacity = 0.7;
        fullscreen_opacity = 1;
        dim_around = 1;
        dim_inactive = true;
        dim_special = 0.3;
        dim_strength = 0.3;
      };

      workspace = [
        "name:terminal, id:1, monitor:HDMI-A-1"
        "name:other, id:4, monitor:HDMI-A-1"
        "name:other2, id:4, monitor:HDMI-A-1"
        "name:misc, id:5, monitor:eDP-1"
        "name:misc2, id:5, monitor:eDP-1"

        "special:browser, id:6, on-created-empty:zen-browser"
        "special:telegram, id:8, on-created-empty:ayugram-desktop"
        "special:music, id:10"
      ];

      windowrulev2 = [
        "workspace name:misc, class:^(zoom|zoom-us|Zoom Workplace|com.github.wwmm.easyeffects)$"
        "workspace special:browser, class:^(google-chrome|google-chrome-stable|com.ayugram.desktop|zen)$, opacity 0.9 0.9"
        "immediate, class:^(mpv)$"
        "float,title:^(Google Chrome.*|Untitled - Google Chrome)$"
        "float,title:^(JamesDSP for Linux.*)$"
        "float,title:^(Untitled - Google Chrome.*)$"
        "float, class:^(org.telegram.desktop)$"
        "float, class:^(pulsemixer|nekoray|hiddify|pavucontrol)$"
        "float, class:^(blueman-manager|nm-applet|nm-connection-editor)$"
        "float, class:^(org.kde.polkit-kde-authentication-agent-1)$"
        "float, class:^(bluetooth-manager|iwgtk)$"
        "float, class:^(1Password)$, size 500 700"
        "float, class:^(com.github.wwmm.easyeffects)$, size 650 400"
        "float, class:^(com-artemchep-keyguard-MainKt)$, size 700 700"
        "float, class:^(xdg-desktop-portal-gtk)$, size 400 400"
        "opacity 0.8 0.8, class:^(org.freedesktop.impl.portal.desktop.gtk|org.freedesktop.impl.portal.desktop.hyprland)$"
      ];

      bind = [
        "SUPER, D, workspace, name:terminal"
        "SUPER SHIFT, D, movetoworkspacesilent, name:terminal"

        "SUPER, 2, workspace, name:other"
        "SUPER SHIFT, 2, movetoworkspacesilent, name:other"

        "SUPER, 3, workspace, name:other2"
        "SUPER SHIFT, 3, movetoworkspacesilent, name:other2"

        "SUPER, 4, workspace, name:misc"
        "SUPER SHIFT, 4, movetoworkspacesilent, name:misc"

        "SUPER, 5, workspace, name:misc2"
        "SUPER SHIFT, 5, movetoworkspacesilent, name:misc2"

        "SUPER, E, togglespecialworkspace, telegram"

        "SUPER, V, togglespecialworkspace, music"

        "SUPER, G, togglespecialworkspace, browser"
        "SUPER SHIFT, G, movetoworkspacesilent, special:browser"

        # window management
        "SUPER, Q, killactive"
        "SUPER, F, togglefloating"
        "SUPER, Space, fullscreen"
        "SUPER, Period, movewindow, mon:+1"
        "SUPER, Comma, movewindow, mon:-1"

        "SUPER SHIFT, H, movewindow, l"
        "SUPER SHIFT, J, movewindow, d"
        "SUPER SHIFT, K, movewindow, u"
        "SUPER SHIFT, L, movewindow, r"

        "SUPER, H, movefocus, l"
        "SUPER, J, movefocus, d"
        "SUPER, K, movefocus, u"
        "SUPER, L, movefocus, r"

        # launches
        "SUPER, Return, exec, ghostty"
        ''CTRL SHIFT, S, exec, grim -g "$(slurp)" - | wl-copy''
        "SUPER, A, exec, anyrun"
      ];

      bindm =
        [ "SUPER, mouse:272, movewindow" "SUPER, mouse:273, resizewindow" ];

      exec-once = [
        "kanata"
        "${../../../../scripts/xdg-portal.sh}"
        "1password"
        "hyprpaper"
        "zen-browser"
        "easyeffects"
      ];
    };
  };
}
