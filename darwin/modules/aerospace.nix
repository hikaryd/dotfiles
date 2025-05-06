{ pkgs, ... }: {
  services.aerospace = {
    enable = false;
    package = pkgs.aerospace;

    settings = {
      after-login-command = [ ];
      after-startup-command = [
        "exec-and-forget open -a /Applications/Spotify.app"
        "exec-and-forget open -a /Applications/Telegram.app"
        "exec-and-forget open -a /Applications/Arc.app"
        "exec-and-forget open -a /Applications/Microsoft Outlook.app"
      ];

      key-mapping.preset = "qwerty";

      enable-normalization-flatten-containers = true;
      enable-normalization-opposite-orientation-for-nested-containers = true;

      accordion-padding = 14;

      default-root-container-layout = "tiles";
      default-root-container-orientation = "horizontal";

      exec-on-workspace-change = [
        "/bin/zsh"
        "-c"
        "${pkgs.sketchybar}/bin/sketchybar --trigger aerospace_workspace_changed FOCUSED_WORKSPACE=$AEROSPACE_FOCUSED_WORKSPACE"
      ];

      on-focus-changed = [
        "exec-and-forget ${pkgs.sketchybar}/bin/sketchybar --trigger front_app_switched"
      ];

      gaps = {
        outer = {
          bottom = 20;
          left = 20;
          right = 20;
          top = 20;
        };
        inner = {
          horizontal = 25;
          vertical = 25;
        };
      };

      on-window-detected = [
        ####### Floating Windows #######
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.apple.finder"; };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "hossin.asaadi.V2Box"; };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.cisco.secureclient.gui"; };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.apple.systempreferences"; };
          run = [ "layout tiling" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "org.pqrs.Karabiner-Elements.Settings"; };
          run = [ "layout floating" ];
        }
        ####### Specific spaces for apps #######
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.mitchellh.ghostty"; };
          run = [ "move-node-to-workspace D" ];
        }

        {
          check-further-callbacks = false;
          "if" = { app-id = "company.thebrowser.Browser"; };
          run = [ "move-node-to-workspace B" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "ru.keepcoder.Telegram"; };
          run = [ "move-node-to-workspace E" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.microsoft.Outlook"; };
          run = [ "move-node-to-workspace M" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "ru.unlimitedtech.express.desktop"; };
          run = [ "move-node-to-workspace M" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.spotify.client"; };
          run = [ "move-node-to-workspace E" ];
        }
      ];

      mode.main.binding = {
        cmd-alt-h = [ ];

        alt-comma = "layout accordion horizontal vertical";

        alt-shift-minus = "resize smart -50";
        alt-shift-equal = "resize smart +50";

        alt-q = "close";
        alt-enter = "exec-and-forget open -n /Applications/Ghostty.app";

        alt-h = "focus left";
        alt-j = "focus down";
        alt-k = "focus up";
        alt-l = "focus right";

        alt-shift-h = "move left";
        alt-shift-j = "move down";
        alt-shift-k = "move up";
        alt-shift-l = "move right";

        cmd-shift-h = "resize width -50";
        cmd-shift-j = "resize height +50";
        cmd-shift-k = "resize height -50";
        cmd-shift-l = "resize width +50";

        alt-shift-0 = "balance-sizes";

        alt-b = "workspace B";
        alt-e = "workspace E";
        alt-m = "workspace M";
        alt-d = "workspace D";
        alt-v = "workspace V";
        alt-p = "workspace P";

        alt-shift-b = [
          "move-node-to-workspace B"
          "exec-and-forget ${pkgs.sketchybar}/bin/sketchybar --trigger space_windows_change"
        ];
        alt-shift-e = [
          "move-node-to-workspace E"
          "exec-and-forget ${pkgs.sketchybar}/bin/sketchybar --trigger space_windows_change"
        ];
        alt-shift-m = [
          "move-node-to-workspace M"
          "exec-and-forget ${pkgs.sketchybar}/bin/sketchybar --trigger space_windows_change"
        ];
        alt-shift-d = [
          "move-node-to-workspace D"
          "exec-and-forget ${pkgs.sketchybar}/bin/sketchybar --trigger space_windows_change"
        ];
        alt-shift-v = [
          "move-node-to-workspace V"
          "exec-and-forget ${pkgs.sketchybar}/bin/sketchybar --trigger space_windows_change"
        ];

        alt-shift-p = [
          "move-node-to-workspace P"
          "exec-and-forget ${pkgs.sketchybar}/bin/sketchybar --trigger space_windows_change"
        ];

        alt-shift-f = "fullscreen";
        alt-shift-tab = "move-workspace-to-monitor --wrap-around next";
        alt-tab = "workspace-back-and-forth";

        cmd-shift-w = "layout floating tiling";
        cmd-shift-s = "layout v_accordion";
        cmd-shift-t = "layout h_accordion";
        cmd-shift-e = "layout tiles horizontal vertical";
        cmd-shift-d = "resize width 1280";

        cmd-shift-semicolon = "mode service";
      };

      mode.service.binding = {
        esc = [ "reload-config" "mode main" ];
        r = [ "flatten-workspace-tree" "mode main" ];
        f = [ "layout floating tiling" "mode main" ];
        backspace = [ "close-all-windows-but-current" "mode main" ];

        "alt-shift-h" = [ "join-with left" "mode main" ];
        "alt-shift-j" = [ "join-with down" "mode main" ];
        "alt-shift-k" = [ "join-with up" "mode main" ];
        "alt-shift-l" = [ "join-with right" "mode main" ];
      };
    };
  };
}

