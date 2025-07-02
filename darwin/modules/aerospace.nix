{ pkgs, ... }: {
  services.aerospace = {
    enable = true;
    package = pkgs.aerospace;

    settings = {
      after-login-command = [ ];
      after-startup-command = [
        "exec-and-forget open -a /Applications/Spotify.app"
        "exec-and-forget open -a /Applications/Telegram.app"
        "exec-and-forget open -a /Applications/Apple Juice.app"
      ];

      key-mapping.preset = "qwerty";

      accordion-padding = 35;

      default-root-container-layout = "tiles";
      default-root-container-orientation = "auto";

      exec-on-workspace-change = [
        "/bin/bash"
        "-c"
        "sketchybar --trigger aerospace_workspace_change AEROSPACE_FOCUSED_WORKSPACE=$AEROSPACE_FOCUSED_WORKSPACE AEROSPACE_PREV_WORKSPACE=$AEROSPACE_PREV_WORKSPACE"
      ];

      on-focused-monitor-changed = [ "move-mouse monitor-lazy-center" ];
      on-focus-changed = [ "move-mouse window-lazy-center" ];
      automatically-unhide-macos-hidden-apps = true;
      enable-normalization-flatten-containers = false;
      enable-normalization-opposite-orientation-for-nested-containers = false;

      gaps = {
        outer = {
          bottom = 20;
          left = 20;
          right = 20;
          top = 70;
        };
        inner = {
          horizontal = 20;
          vertical = 20;
        };
      };

      on-window-detected = [
        ####### xFloating Windows (manage=off) #######
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.apple.finder"; };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = {
            app-id = "com.kagi.kagimacOS";
            window-title-regex-substring =
              "^(General|(Tab|Password|Website|Extension)s|AutoFill|Se(arch|curity)|Privacy|Advance)$";
          };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "Calculator"; };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "me.tdinh.devutils-setapp"; };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "su.ffg.happ.plus"; };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.cisco.secureclient.gui"; };
          run = [ "layout floating" ];
        }

        ####### Specific spaces for apps #######
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.apple.Safari"; };
          run = [ "move-node-to-workspace 1" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.brave.Browser"; };
          run = [ "move-node-to-workspace 1" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "app.zen-browser.zen"; };
          run = [ "move-node-to-workspace 1" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.kagi.kagimacOS"; };
          run = [ "move-node-to-workspace 1" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.mitchellh.ghostty"; };
          run = [ "layout floating" "move-node-to-workspace 2" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.github.wez.wezterm"; };
          run = [ "move-node-to-workspace 2" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.todesktop.230313mzl4w4u92"; };
          run = [ "move-node-to-workspace 4" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.apple.iWork.Pages"; };
          run = [ "move-node-to-workspace 4" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.apple.iCal"; };
          run = [ "move-node-to-workspace 7" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "ru.keepcoder.Telegram"; };
          run = [ "move-node-to-workspace 3" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.microsoft.Outlook"; };
          run = [ "move-node-to-workspace 7" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "ru.unlimitedtech.express.desktop"; };
          run = [ "move-node-to-workspace 7" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.spotify.client"; };
          run = [ "move-node-to-workspace 3" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.anthropic.claudefordesktop"; };
          run = [ "move-node-to-workspace 5" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "kontur.talk"; };
          run = [ "move-node-to-workspace 6" ];
        }
      ];

      workspace-to-monitor-force-assignment = {
        "1" = "secondary";
        "2" = "main";
        "3" = "main";
        "4" = "main";
        "5" = "secondary";
        "6" = "secondary";
        "7" = "secondary";
        "8" = "main";
      };

      mode.main.binding = {
        alt-shift-minus = "resize smart -100";
        alt-shift-equal = "resize smart +100";

        alt-q = "close";

        alt-h = "focus left";
        alt-j = "focus down";
        alt-k = "focus up";
        alt-l = "focus right";

        alt-shift-h = "move left";
        alt-shift-j = "move down";
        alt-shift-k = "move up";
        alt-shift-l = "move right";

        alt-shift-0 = "balance-sizes";

        alt-1 = "workspace 1";
        alt-2 = "workspace 2";
        alt-3 = "workspace 3";
        alt-4 = "workspace 4";
        alt-5 = "workspace 5";
        alt-6 = "workspace 6";
        alt-7 = "workspace 7";
        alt-8 = "workspace 8";

        alt-shift-1 = [ "move-node-to-workspace 1" ];
        alt-shift-2 = [ "move-node-to-workspace 2" ];
        alt-shift-3 = [ "move-node-to-workspace 3" ];
        alt-shift-4 = [ "move-node-to-workspace 4" ];
        alt-shift-5 = [ "move-node-to-workspace 5" ];
        alt-shift-6 = [ "move-node-to-workspace 6" ];
        alt-shift-7 = [ "move-node-to-workspace 7" ];
        alt-shift-8 = [ "move-node-to-workspace 8" ];

        alt-shift-tab = "move-workspace-to-monitor --wrap-around next";
        alt-tab = "workspace-back-and-forth";

        alt-shift-f = "fullscreen";

        cmd-shift-e = "layout tiles h_accordion tiling";
        cmd-shift-r = [ "flatten-workspace-tree" "mode main" ];
        cmd-shift-f = [ "layout floating tiling" ];
      };

      mode.service.binding = {
        esc = [ "reload-config" "mode main" ];
        backspace = [ "close-all-windows-but-current" "mode main" ];

        "alt-shift-h" = [ "join-with left" "mode main" ];
        "alt-shift-j" = [ "join-with down" "mode main" ];
        "alt-shift-k" = [ "join-with up" "mode main" ];
        "alt-shift-l" = [ "join-with right" "mode main" ];
      };
    };
  };
}
