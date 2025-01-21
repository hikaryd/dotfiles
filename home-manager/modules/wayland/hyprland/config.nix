{ inputs, config, pkgs, ... }: {
  wayland.windowManager.hyprland = {
    enable = true;
    package = config.lib.nixGL.wrap
      inputs.hyprland.packages.${pkgs.stdenv.hostPlatform.system}.hyprland;
    systemd.enable = true;
    xwayland.enable = true;

    settings = {
      monitor = [
        "eDP-1, 2560x1600@120, -2560x0, 1.25"
        "desc:AOC Q27G2G3R3B RTEMAHA004734, 2560x1440@120, 0x0, 1.25"
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
        "workspace name:dev-terminal, class:^(ghostty)$"
        "workspace name:conf-terminal, class:^(ghostty)$"
        "workspace name:terminal, class:^(ghostty)$"
        "workspace name:database, class:^(DBeaver)$"
        "workspace name:other, class:^(zoom)$"

        "workspace special:telegram, class:^(org.telegram.desktop)$"
        "workspace special:audio, class:^(pulsemixer)$"
        "workspace special:audio, class:^(com.github.wwmm.easyeffects)$"
        "workspace special:music, class:^(YouTube Music)$"
        "workspace special:vpn, class:^(nekoray)$"
        "workspace special:ai_ide, class:^(codium)$"

        "immediate, class:^(mpv)$"
        "float,title:^(Google Chrome)(.*)$"
        "float,title:^(Untitled - Google Chrome)(.*)$"

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

        "float, class:^(com.github.th_ch.youtube_music)$"
        "size 800 1000, class:^(com.github.th_ch.youtube_music)$"
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

        "opacity 0.9 0.9,class:^(zen)$"
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

      cursor = { no_hardware_cursors = true; };
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

        "WLR_DRM_NO_ATOMIC,1"
        "MOZ_ENABLE_WAYLAND,1"
        "GDK_SCALE,1"
        "SDL_VIDEODRIVER,wayland"
        "ELECTRON_ENABLE_WAYLAND,1"
        "ELECTRON_OZONE_PLATFORM_HINT,wayland"
        "WINIT_UNIX_BACKEND,wayland"
        "WLR_RENDERER,vulkan"
        "WLR_RENDERER_ALLOW_SOFTWARE,1"

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
        rounding = 0;
        shadow = {
          enabled = true;
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

      workspace = [
        # Основные рабочие пространства
        "name:browser, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, on-created-empty:zen"
        "name:dev-terminal, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, on-created-empty:ghostty, default:true"
        "name:conf-terminal, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, on-created-empty:ghostty"
        "name:terminal, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, on-created-empty:ghostty"
        "name:database, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, on-created-empty:dbeaver, default:true"
        "name:other, monitor:desc:AOC Q27G2G3R3B RTEMAHA004734, default:true"
        "name:other2, monitor:eDP-1, default:true"

        # Специальные рабочие пространства
        "special:telegram, on-created-empty:telegram-desktop, default:true"
        "special:misc, default:true"
        "special:audio, on-created-empty:audio, default:true"
        "special:music, on-created-empty:youtube-music, default:true"
        "special:vpn, on-created-empty:nekoray, default:true"
        "special:browser2, on-created-empty:zen"
      ];

      "$mainMod" = "SUPER";
      "$base" = "SUPER";
      "$window" = "SUPER SHIFT"; # Управление окнами
      "$layout" = "SUPER ALT"; # Управление расположением
      "$system" = "SUPER CTRL"; # Системные функции
      "$launch" = "SUPER SHIFT ALT"; # Запуск приложений
      bind = [
        # ===== Основные рабочие пространства =====
        "$base, B, workspace, name:browser"
        "$base, D, workspace, name:dev-terminal"
        "$base, C, workspace, name:conf-terminal"
        "$base, T, workspace, name:terminal"
        "$base, M, workspace, name:misc"
        "$base, O, workspace, name:other"
        "$system, O, workspace, name:other2"
        "$system, M, workspace, name:database"

        # ===== Управление окнами =====
        "$window, B, movetoworkspacesilent, name:browser"
        "$window, D, movetoworkspacesilent, name:dev-terminal"
        "$window, C, movetoworkspacesilent, name:conf-terminal"
        "$window, T, movetoworkspacesilent, name:terminal"
        "$window, M, movetoworkspacesilent, name:misc"
        "$window, O, movetoworkspacesilent, name:other"

        "$window, Q, killactive"
        "$window, Space, togglefloating"
        "$window, F, fullscreen"
        "$window, Period, movewindow, mon:+1"
        "$window, Comma, movewindow, mon:-1"

        "$window, H, movewindow, l"
        "$window, J, movewindow, d"
        "$window, K, movewindow, u"
        "$window, L, movewindow, r"
        "$base, h, movefocus, l"
        "$base, l, movefocus, r"
        "$base, k, movefocus, u"
        "$base, j, movefocus, d"

        # ===== Системные функции =====
        # Системное управление
        "$system, L, exec, hyprlock"

        # Специальные рабочие пространства
        "$system, P, togglespecialworkspace, music"
        "$system, A, togglespecialworkspace, audio"
        "$system, V, togglespecialworkspace, vpn"
        "$system, T, togglespecialworkspace, telegram"
        "$system, G, togglespecialworkspace, browser2"

        # Системные панели и меню
        "$system, S, exec, hyprpanel t dashboardmenu"
        "$system, B, exec, hyprpanel t bluetoothmenu"
        "$system, N, exec, hyprpanel t notificationsmenu"
        "$system, W, exec, hyprpanel t networkmenu"

        # ===== Запуск приложений =====
        "$base, Return, exec, ghostty"
        "$launch, S, exec, ${../../../../scripts/snapshot.sh}"
        "$base, A, exec, anyrun"
        "$launch, B, exec, GDK_BACKEND=x11 dbeaver"
      ];

      bindm = [
        "$mainMod, mouse:272, movewindow"
        "$mainMod, mouse:273, resizewindow"
      ];

      exec-once = [
        "${pkgs.polkit_gnome}/libexec/polkit-gnome-authentication-agent-1"
        "nekoray"
        "dbus-update-activation-environment --systemd WAYLAND_DISPLAY XDG_CURRENT_DESKTOP"
        "${../../../../scripts/xdg-portal.sh}"
        "easyeffects"
        "kitty --class pulsemixer -- pulsemixer"
        "blueman-applet"
        "wl-paste -t text -w xclip -selection clipboard --watch cliphist store"
        "wl-paste --type image --watch cliphist store"
      ];
    };
  };

}

