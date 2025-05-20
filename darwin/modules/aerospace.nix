{ pkgs, ... }: {
  services.aerospace = {
    enable = false;
    package = pkgs.aerospace;

    settings = {
      after-login-command = [ ];
      after-startup-command = [
        "exec-and-forget open -a /Applications/Spotify.app"
        "exec-and-forget open -a /Applications/Telegram.app"
        "exec-and-forget open -a /Applications/Microsoft Outlook.app"
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
          bottom = 6;
          left = 6;
          right = 6;
          top = 6;
        };
        inner = {
          horizontal = 12;
          vertical = 12;
        };
      };

      on-window-detected = [
        ####### Floating Windows (manage=off) #######
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
          "if" = { app-id = "com.mitchellh.ghostty"; };
          run = [ "layout floating" "move-node-to-workspace D" ];
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
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.openai.chat"; };
          run = [ "move-node-to-workspace V" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "kontur.talk"; };
          run = [ "move-node-to-workspace P" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.setapp.DesktopClient"; };
          run = [ "move-node-to-workspace P" ];
        }
      ];

      workspace-to-monitor-force-assignment = {
        "B" = "main";
        "E" = "main";
        "M" = "main";
        "D" = "main";
        "V" = [ "secondary" "dell" ];
        "P" = "main";
        "O" = "main";
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

        alt-b = "workspace B";
        alt-e = "workspace E";
        alt-m = "workspace M";
        alt-d = "workspace D";
        alt-v = "workspace V";
        alt-p = "workspace P";
        alt-o = "workspace O";

        alt-shift-b = [ "move-node-to-workspace B" ];
        alt-shift-e = [ "move-node-to-workspace E" ];
        alt-shift-m = [ "move-node-to-workspace M" ];
        alt-shift-d = [ "move-node-to-workspace D" ];
        alt-shift-v = [ "move-node-to-workspace V" ];
        alt-shift-p = [ "move-node-to-workspace P" ];

        alt-shift-o = [ "move-node-to-workspace O" ];

        alt-shift-f = "fullscreen";
        alt-shift-tab = "move-workspace-to-monitor --wrap-around next";
        alt-tab = "workspace-back-and-forth";

        cmd-shift-w = "layout floating tiling";
        cmd-shift-s = "layout v_accordion";
        cmd-shift-t = "layout h_accordion";
        cmd-shift-e = "layout tiles horizontal vertical";
        cmd-shift-d = "resize width 1280";

        cmd-shift-semicolon = "mode service";
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
