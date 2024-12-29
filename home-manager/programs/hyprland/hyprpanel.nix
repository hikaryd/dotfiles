{ pkgs, ... }: {
  home.packages = [ pkgs.hyprpanel ];

  wayland.windowManager.hyprland.extraConfig = ''
    exec-once = hyprpanel
  '';

  xdg.configFile."hyprpanel/config.json" = {
    text = builtins.toJSON {
      theme = "catppuccin_mocha_split";
      "bar.customModules.updates.pollingInterval" = 1440000;
      "theme.bar.scaling" = 70;
      "theme.notification.scaling" = 70;
      "theme.osd.scaling" = 70;
      "theme.bar.menus.menu.dashboard.scaling" = 70;
      "theme.bar.menus.menu.dashboard.confirmation_scaling" = 70;
      "theme.bar.menus.menu.media.scaling" = 70;
      "theme.bar.menus.menu.volume.scaling" = 70;
      "theme.bar.menus.menu.network.scaling" = 70;
      "theme.bar.menus.menu.bluetooth.scaling" = 70;
      "theme.bar.menus.menu.battery.scaling" = 70;
      "theme.bar.menus.menu.clock.scaling" = 70;
      "theme.bar.menus.menu.notifications.scaling" = 70;
      "theme.tooltip.scaling" = 70;
      "theme.bar.menus.popover.scaling" = 70;
      "theme.bar.menus.menu.power.scaling" = 70;
      scalingPriority = "hyprland";
      "menus.transition" = "crossfade";
      "wallpaper.pywal" = false;
      "wallpaper.image" =
        "/home/hikary/dotfiles/wallpapers/minimalist-moon-night-mountains.jpg";
      "theme.matugen" = false;
      "theme.bar.transparent" = true;
      "theme.bar.buttons.radius" = "1.5em";
      tear = true;
      "theme.bar.buttons.modules.ram.enableBorder" = true;
      "theme.bar.buttons.modules.cpu.enableBorder" = true;
      "theme.bar.buttons.modules.cpuTemp.enableBorder" = true;
      "bar.customModules.netstat.dynamicIcon" = true;
      "theme.bar.buttons.modules.netstat.enableBorder" = true;
      "theme.bar.buttons.modules.kbLayout.enableBorder" = true;
      "menus.dashboard.powermenu.avatar.name" = "hikary";
      "menus.dashboard.controls.enabled" = false;
      "menus.dashboard.shortcuts.enabled" = true;
      "menus.dashboard.directories.enabled" = false;
      "theme.bar.buttons.style" = "split";
      "bar.layouts" = {
        "0" = {
          left = [ "dashboard" "workspaces" ];
          middle = [ "media" ];
          right =
            [ "volume" "network" "bluetooth" "battery" "clock" "notifications" ];
        };
        "1" = {
          left = [ "dashboard" "workspaces" ];
          middle = [ "media" ];
          right =
            [ "volume" "network" "bluetooth" "battery" "clock" "notifications" ];
        };
      };
    };
    force = true;
  };
}
