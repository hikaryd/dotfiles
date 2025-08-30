{ ... }: {
  services.aerospace = {
    enable = true;
    settings = {
      after-login-command = [ ];
      after-startup-command = [ ];

      key-mapping.preset = "qwerty";

      accordion-padding = 0;

      default-root-container-layout = "tiles";
      default-root-container-orientation = "auto";

      exec-on-workspace-change = [ ];

      on-focused-monitor-changed = [ "move-mouse monitor-lazy-center" ];
      on-focus-changed = [ "move-mouse window-lazy-center" ];
      automatically-unhide-macos-hidden-apps = true;
      enable-normalization-flatten-containers = false;
      enable-normalization-opposite-orientation-for-nested-containers = false;

      gaps = {
        outer = {
          bottom = 0;
          left = 0;
          right = 0;
          top = 0;
        };
        inner = {
          horizontal = 0;
          vertical = 0;
        };
      };

      on-window-detected = [
        ####### xFloating Windows (manage=off) #######
        {
          check-further-callbacks = false;
          "if" = {
            app-id = "com.brave.Browser";
            window-title-regex-substring =
              "^(General|(Tab|Password|Website|Extension)s|AutoFill|Se(arch|curity)|Privacy|Advance)$";
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
          "if" = { app-id = "org.kde.kdeconnect"; };
          run = [ "layout floating" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.liguangming.Shadowrocket"; };
          run = [ "layout floating" ];
        }

        ####### Specific spaces for apps #######
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.apple.systempreferences"; };
          run = [ "layout tiling" "move-node-to-workspace e" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.brave.Browser"; };
          run = [ "move-node-to-workspace 1" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.mitchellh.ghostty"; };
          run = [ "layout tiling" "move-node-to-workspace d" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.electron.dockerdesktop"; };
          run = [ "move-node-to-workspace q" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "org.jkiss.dbeaver.core.product"; };
          run = [ "move-node-to-workspace q" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.openai.chat"; };
          run = [ "move-node-to-workspace 2" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.apple.iWork.Pages"; };
          run = [ "move-node-to-workspace 4" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "ru.keepcoder.Telegram"; };
          run = [ "move-node-to-workspace 3" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.apple.finder"; };
          run = [ "move-node-to-workspace e" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "ch.protonmail.desktop"; };
          run = [ "move-node-to-workspace 3" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.apple.AppStore"; };
          run = [ "move-node-to-workspace e" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.spotify.client"; };
          run = [ "move-node-to-workspace 3" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "kontur.talk"; };
          run = [ "move-node-to-workspace t" ];
        }
      ];

      workspace-to-monitor-force-assignment = {
        "1" = "main";
        "2" = "main";
        "3" = "secondary";
        "4" = "main";
        "q" = "secondary";
        "w" = "secondary";
        "e" = "secondary";
        "d" = "main";
        "t" = "secondary";
      };

      mode.main.binding = {
        alt-shift-minus = "resize smart -100";
        alt-shift-equal = "resize smart +100";

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
        alt-q = "workspace q";
        alt-w = "workspace w";
        alt-e = "workspace e";
        alt-d = "workspace d";
        alt-t = "workspace t";

        alt-shift-1 = [ "move-node-to-workspace 1" ];
        alt-shift-2 = [ "move-node-to-workspace 2" ];
        alt-shift-3 = [ "move-node-to-workspace 3" ];
        alt-shift-4 = [ "move-node-to-workspace 4" ];
        alt-shift-q = [ "move-node-to-workspace q" ];
        alt-shift-w = [ "move-node-to-workspace w" ];
        alt-shift-e = [ "move-node-to-workspace e" ];
        alt-shift-d = [ "move-node-to-workspace d" ];
        alt-shift-t = [ "move-node-to-workspace t" ];

        alt-shift-tab = "move-workspace-to-monitor --wrap-around next";
        alt-tab = "workspace-back-and-forth";

        alt-shift-f = "fullscreen";

        cmd-shift-l = "join-with right";
        cmd-shift-h = "join-with left";
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
