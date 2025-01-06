{ pkgs, ... }: {
  home.packages = with pkgs;
    [
      (pkgs.writeShellScriptBin "hyprpanel" ''
        ${pkgs.nixgl.nixGLMesa}/bin/nixGLMesa ${hyprpanel}/bin/hyprpanel "$@"
      '')
    ];

  wayland.windowManager.hyprland.extraConfig = ''
    exec-once = hyprpanel
  '';

  xdg.configFile."hyprpanel/config.json" = {
    text = builtins.toJSON {
      theme = "catppuccin_mocha_split";
      tear = true;
      scalingPriority = "hyprland";
      "bar.customModules.updates.pollingInterval" = 1440000;
      "menus.dashboard.controls.enabled" = false;
      "menus.dashboard.directories.enabled" = false;
      "menus.dashboard.powermenu.avatar.name" = "hikary";
      "menus.dashboard.shortcuts.enabled" = true;
      "menus.transition" = "crossfade";
      "theme.bar.buttons.modules.cpu.enableBorder" = true;
      "theme.bar.buttons.modules.cpuTemp.enableBorder" = true;
      "theme.bar.buttons.modules.kbLayout.enableBorder" = true;
      "theme.bar.buttons.modules.netstat.enableBorder" = true;
      "theme.bar.buttons.modules.ram.enableBorder" = true;
      "theme.bar.buttons.radius" = "1.5em";
      "theme.bar.buttons.style" = "split";
      "theme.bar.menus.menu.battery.scaling" = 70;
      "theme.bar.menus.menu.bluetooth.scaling" = 70;
      "theme.bar.menus.menu.clock.scaling" = 70;
      "theme.bar.menus.menu.dashboard.confirmation_scaling" = 70;
      "theme.bar.menus.menu.dashboard.scaling" = 70;
      "theme.bar.menus.menu.media.scaling" = 70;
      "theme.bar.menus.menu.network.scaling" = 70;
      "theme.bar.menus.menu.notifications.scaling" = 70;
      "theme.bar.menus.menu.power.scaling" = 70;
      "theme.bar.menus.menu.volume.scaling" = 70;
      "theme.bar.menus.popover.scaling" = 70;
      "theme.bar.scaling" = 70;
      "theme.bar.transparent" = true;
      "theme.matugen" = false;
      "theme.notification.scaling" = 70;
      "theme.osd.scaling" = 70;
      "theme.tooltip.scaling" = 70;
      "wallpaper.image" =
        "/home/hikary/dotfiles/wallpapers/minimalist-moon-night-mountains.jpg";
      "wallpaper.pywal" = false;
      "bar.workspaces.show_icons" = true;
      "bar.workspaces.show_numbered" = false;
      "bar.workspaces.numbered_active_indicator" = "underline";
      "bar.workspaces.workspaces" = 10;
      "bar.workspaces.monitorSpecific" = false;
      "bar.workspaces.hideUnoccupied" = false;
      "bar.workspaces.workspaceMask" = false;
      "bar.workspaces.reverse_scroll" = false;
      "bar.workspaces.ignored" = "-\\d+";
      "bar.workspaces.showApplicationIcons" = false;
      "bar.workspaces.showWsIcons" = true;
      "bar.layouts" = {
        "0" = {
          left = [ "dashboard" "workspaces" ];
          middle = [ "media" ];
          right = [
            "volume"
            "network"
            "bluetooth"
            "battery"
            "clock"
            "notifications"
          ];
        };
        "1" = {
          left = [ "dashboard" "workspaces" ];
          middle = [ "media" ];
          right = [
            "volume"
            "network"
            "bluetooth"
            "battery"
            "clock"
            "notifications"
          ];
        };
      };
    };
    force = true;
  };
}
