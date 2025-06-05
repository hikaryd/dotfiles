{ pkgs, ... }: {
  services.aerospace = {
    enable = true;
    package = pkgs.aerospace;

    settings = {
      after-login-command = [ ];
      after-startup-command = [
        "exec-and-forget open -a /Applications/Spotify.app"
        "exec-and-forget open -a /Applications/Telegram.app"
        "exec-and-forget open -a /Applications/Ghostty.app"
        "exec-and-forget open -a /Applications/Safari.app"
        "exec-and-forget open -a /Applications/Barik.app"
      ];

      key-mapping.preset = "qwerty";

      enable-normalization-flatten-containers = true;
      enable-normalization-opposite-orientation-for-nested-containers = true;

      accordion-padding = 5;

      default-root-container-layout = "tiles";
      default-root-container-orientation = "horizontal";

      exec-on-workspace-change = [ ];

      on-focus-changed = [ "move-mouse window-lazy-center" ];

      gaps = {
        outer = {
          bottom = 12;
          left = 12;
          right = 12;
          top = 52;
        };
        inner = {
          horizontal = 12;
          vertical = 12;
        };
      };

      on-window-detected = [
        ####### xFloating Windows (manage=off) #######
        {
          check-further-callbacks = false;
          "if" = { app-id = "System Settings"; };
          run = [ "layout floating" ];
        }
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
          "if" = {
            app-id = "company.thebrowser.Browser";
            window-title-regex-substring =
              "^(General|(Tab|Password|Website|Extension)s|AutoFill|Se(arch|curity)|Privacy|Advance)$";
          };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "System Preferences"; };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "App Store"; };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "Activity Monitor"; };
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
          "if" = { app-id = "Dictionary"; };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "mpv"; };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { window-title-regex-substring = "Software Update"; };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "su.ffg.happ.plus"; };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = {
            app-id = "System Information";
            window-title-regex-substring = "About This Mac";
          };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.cisco.secureclient.gui"; };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.kis.keepitshot-setapp"; };
          run = [ "layout floating" ];
        }

        ####### Specific spaces for apps #######
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.apple.Safari"; };
          run = [ "layout floating" "move-node-to-workspace 1" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.mitchellh.ghostty"; };
          run = [ "layout floating" "move-node-to-workspace 2" ];
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
          "if" = { app-id = "kontur.talk"; };
          run = [ "move-node-to-workspace 5" ];
        }
      ];

      workspace-to-monitor-force-assignment = {
        "1" = "main";
        "2" = "main";
        "3" = "main";
        "4" = "main";
        "5" = "main";
        "6" = "main";
        "7" = "main";
      };

      mode.main.binding = {
        alt-shift-minus = "resize smart -100";
        alt-shift-equal = "resize smart +100";

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

        alt-shift-0 = "balance-sizes";

        alt-1 = "workspace 1";
        alt-2 = "workspace 2";
        alt-3 = "workspace 3";
        alt-4 = "workspace 4";
        alt-5 = "workspace 5";
        alt-6 = "workspace 6";
        alt-7 = "workspace 7";

        alt-shift-1 = [ "move-node-to-workspace 1" ];
        alt-shift-2 = [ "move-node-to-workspace 2" ];
        alt-shift-3 = [ "move-node-to-workspace 3" ];
        alt-shift-4 = [ "move-node-to-workspace 4" ];
        alt-shift-5 = [ "move-node-to-workspace 5" ];
        alt-shift-6 = [ "move-node-to-workspace 6" ];
        alt-shift-7 = [ "move-node-to-workspace 7" ];

        alt-shift-f = "fullscreen";
        alt-shift-tab = "move-workspace-to-monitor --wrap-around next";
        alt-tab = "workspace-back-and-forth";

        cmd-shift-w = "layout floating tiling";
        cmd-shift-r = "layout h_accordion";
        cmd-shift-e = "layout tiles horizontal vertical";

        cmd-shift-semicolon = "mode service";
        cmd-shift-t = [ "resize width 700" ];
        cmd-shift-f = [ "layout floating tiling" "mode main" ];
      };

      mode.service.binding = {
        esc = [ "reload-config" "mode main" ];
        r = [ "flatten-workspace-tree" "mode main" ];
        backspace = [ "close-all-windows-but-current" "mode main" ];

        "alt-shift-h" = [ "join-with left" "mode main" ];
        "alt-shift-j" = [ "join-with down" "mode main" ];
        "alt-shift-k" = [ "join-with up" "mode main" ];
        "alt-shift-l" = [ "join-with right" "mode main" ];
      };
    };
  };
}
