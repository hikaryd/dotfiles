{ inputs, config, pkgs, ... }: {
  home.file.".local/share/wayland-sessions/hyprland.desktop".text = ''
    [Desktop Entry]
    Name=Hyprland
    Comment=A dynamic tiling Wayland compositor
    Exec=${inputs.hyprland.packages.${pkgs.stdenv.hostPlatform.system}.hyprland}
    Type=Application
    Keywords=tiling;compositor;wayland;
  '';

  wayland.windowManager.hyprland = {
    enable = true;
    package = config.lib.nixGL.wrap
      inputs.hyprland.packages.${pkgs.stdenv.hostPlatform.system}.hyprland;
    systemd.enable = true;
    xwayland.enable = true;
    settings = {
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

      windowrulev2 = [
        "workspace name:dev-terminal, class:^(ghostty)$"
        "workspace name:dev-terminal, class:^(neovide)$"
        "workspace name:conf-terminal, class:^(ghostty)$"
        "workspace name:terminal, class:^(ghostty)$"
        "workspace name:database, class:^(DBeaver)$"
        "workspace name:database, class:^(beekeeper-studio)$"
        "workspace name:other, class:^(zoom)$"
        "workspace name:other, class:^(zoom-us)$"
        "workspace name:other, class:^(Zoom Workplace)$"
        "workspace name:other, class:^(codium)$"

        "workspace special:telegram, class:^(org.telegram.desktop)$"
        "workspace special:telegram, class:^(com.ayugram.desktop)$"
        "workspace special:music, class:^(com.mark.music)$"
        "workspace special:vpn, class:^(nekoray)$"

        "immediate, class:^(mpv)$"

        "float,title:^(Google Chrome)(.*)$"
        "float,title:^(JamesDSP for Linux)(.*)$"
        "float,title:^(Untitled - Google Chrome)(.*)$"
        "float, class:^(org.telegram.desktop)$"
        "float, class:^(com.ayugram.desktop)$"
        "float, class:^(pulsemixer)$"
        "float, class:^(nekoray)$"
        "float, class:^(pavucontrol)$"
        "float, class:^(blueman-manager)$"
        "float, class:^(nm-applet)$"
        "float, class:^(nm-connection-editor)$"
        "float, class:^(org.kde.polkit-kde-authentication-agent-1)$"
        "float, class:^(bluetooth-manager)$"
        "float, class:^(iwgtk)$"
        "float, class:^(pulsemixer)$"

        "float, class:^(com.mark.bluetui)$"
        "center, class:^(com.mark.monitor)$"
        "size 400 600, class:^(com.mark.bluetui)$"

        "float, class:^(com.mark.monitor)$"
        "center, class:^(com.mark.monitor)$"
        "size 1000 606, class:^(com.mark.monitor)$"

        "float, class:^(com.mark.term-launcher)$"
        "center, class:^(com.mark.term-launcher)$"
        "size 300 500, class:^(com.mark.term-launcher)$"

        "float, class:^(com.mark.pulsemixer)$"
        "center, class:^(com.mark.pulsemixer)$"
        "size 600 300, class:^(com.mark.pulsemixer)$"

        "float, class:^(com.ayugram.desktop)$"
        "center, class:^(com.ayugram.desktop)$"
        "size 700 1000, class:^(com.ayugram.desktop)$"

        "float, class:^(com.github.wwmm.easyeffects)$"
        "center, class:^(com.github.wwmm.easyeffects)$"
        "size 650 400, class:^(com.github.wwmm.easyeffects)$"

        "float, class:^(com.mark.music)$"
        "center, class:^(com.mark.music)$"
        "size 700 1000, class:^(com.mark.music)$"

        "float, class:^(com-artemchep-keyguard-MainKt)$"
        "center, class:^(com-artemchep-keyguard-MainKt)$"
        "size 700 700, class:^(com-artemchep-keyguard-MainKt)$"

        "float, class:^(xdg-desktop-portal-gtk)$"
        "center, class:^(xdg-desktop-portal-gtk)$"
        "size 400 400, class:^(xdg-desktop-portal-gtk)$"

        "opacity 0.80 0.80, class:^(org.freedesktop.impl.portal.desktop.gtk)$"
        "opacity 0.80 0.80, class:^(org.freedesktop.impl.portal.desktop.hyprland)$"
        "opacity 0.9 0.9,class:^(neovide)$"
        "opacity 0.9 0.9,class:^(zen)$"
        "opacity 0.9 0.9,class:^(Min)$"
        "animation off, class:^(flameshot)$"
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
        bezier = [ "myBezier, .5, .25, 0, 1" ];
        animation = [
          "windows, 1, 2.5, myBezier, popin 80%"
          "border, 1, 2.5, myBezier"
          "fade, 1, 2.5, myBezier"
          "workspaces, 1, 2.5, myBezier, slidefade 20%"
        ];
      };

      decoration = {
        rounding = 14;
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
        inactive_opacity = 1;
        fullscreen_opacity = 1;
        dim_around = 1;
        dim_inactive = true;
        dim_special = 0.3;
        dim_strength = 0.3;
      };

      workspace = [
        # Основные рабочие пространства
        "name:browser, monitor:HDMI-A-1, on-created-empty:min-browser"
        "name:dev-terminal, monitor:HDMI-A-1, on-created-empty:ghostty"
        "name:conf-terminal, monitor:HDMI-A-1, on-created-empty:ghostty"
        "name:terminal, monitor:HDMI-A-1, on-created-empty:ghostty, default:true"
        "name:database, monitor:HDMI-A-1, on-created-empty:beekeeper-studio, default:true"
        "name:other, monitor:HDMI-A-1, default:true"
        "special:xreal,monitor:xreal"

        # Специальные рабочие пространства
        "special:telegram, on-created-empty:ayugram-desktop, default:true"
        "special:ai, on-created-empty:cherry-studio, default:true"
        "special:misc, default:true"
        ''
          special:music, default:true, on-create-empty:ghostty --class=com.mark.music -e nu -c "echo 1; mgraftcp --socks5 127.0.0.1:2080 ytermusic"
        ''
        "special:vpn, on-created-empty:nekoray, default:true"
        "special:browser2, on-created-empty:min-browser"
      ];

      "$mainMod" = "SUPER";
      "$base" = "SUPER";
      "$window" = "SUPER SHIFT"; # Управление окнами
      "$layout" = "SUPER ALT"; # Управление расположением
      "$system" = "SUPER CTRL"; # Системные функции
      "$launch" = "SUPER SHIFT ALT"; # Запуск приложений
      bind = [
        # ===== Основные рабочие пространства =====
        "$base, R, workspace, name:browser"
        "$window, R, movetoworkspacesilent, name:browser"

        "$base, G, workspace, name:dev-terminal"
        "$window, G, movetoworkspacesilent, name:dev-terminal"

        "$base, T, workspace, name:conf-terminal"
        "$window, T, movetoworkspacesilent, name:conf-terminal"

        "$base, V, workspace, name:terminal"
        "$window, V, movetoworkspacesilent, name:terminal"

        "$base, C, workspace, name:misc"
        "$window, C, movetoworkspacesilent, name:misc"

        "$base, O, workspace, name:other"
        "$window, O, movetoworkspacesilent, name:other"

        "$base, 4, workspace, name:database"

        "$base, E, togglespecialworkspace, telegram"
        "$base, 5, togglespecialworkspace, music"
        "$base, 6, togglespecialworkspace, vpn"
        "$base, B, togglespecialworkspace, browser2"

        # ===== Управление окнами =====

        "$window, F, killactive"
        "$window, Space, togglefloating"
        "$window, Period, movewindow, mon:+1"
        "$window, Comma, movewindow, mon:-1"

        "$window, H, movewindow, l"
        "$window, J, movewindow, d"
        "$window, K, movewindow, u"
        "$window, L, movewindow, r"
        "$system, h, movefocus, l"
        "$system, l, movefocus, r"
        "$system, k, movefocus, u"
        "$system, j, movefocus, d"

        # ===== Запуск приложений =====
        "$base, Return, exec, ghostty"
        "$launch, C, exec, grimblast save area - | satty --filename - --fullscreen --copy-command wl-copy"
        "$launch, O, exec, ${../../../../scripts/ai_refactor_clipboard}"
        "$base, A, exec, anyrun"
        # "$base, A, exec, sh -c 'exec 3>/dev/null; exec ghostty --class=com.mark.term-launcher -e sway-launcher-desktop'"
        "$launch, B, exec, ghostty --class=com.mark.bluetui -e bluetui"
        "$launch, P, exec, ghostty --class=com.mark.pulsemixer -e pulsemixer"
      ];

      bindm = [
        "$mainMod, mouse:272, movewindow"
        "$mainMod, mouse:273, resizewindow"
      ];
      exec =
        [ "pkill hyprpaper && hyprpaper" "pkill waybar && waybar" "kanata" ];

      exec-once = [
        "kanshi"
        "hypridle"
        "dunst"

        "nekoray"

        "nm-applet"
        "blueman-applet"

        ''
          sleep 10 && ghostty --class=com.mark.music -e nu -c "echo 1; mgraftcp --socks5 127.0.0.1:2080 ytermusic"
        ''

        "${../../../../scripts/xdg-portal.sh}"

        "easyeffects --gapplication-service"
        "jamesdsp"

        "${pkgs.polkit_gnome}/libexec/polkit-gnome-authentication-agent-1"
        "dbus-update-activation-environment --systemd WAYLAND_DISPLAY XDG_CURRENT_DESKTOP"

        "wl-paste -t text -w xclip -selection clipboard --watch cliphist store"
        "wl-paste --type image --watch cliphist store"
      ];
    };
  };
}
