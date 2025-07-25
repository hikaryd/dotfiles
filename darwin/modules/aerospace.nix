{ pkgs, lib, ... }: {
  services.aerospace = {
    enable = lib.mkForce false;
    package = pkgs.aerospace;

    settings = {
      after-login-command = [ ];
      after-startup-command = [
        "exec-and-forget open -a /Applications/Spotify.app"
        "exec-and-forget open -a /Applications/Telegram.app"
        "exec-and-forget open -a /Applications/Brave Browser.app"
      ];

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
          bottom = 25;
          left = 25;
          right = 25;
          top = 25;
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
          "if" = {
            app-id = "org.mozilla.firefox";
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
          "if" = { app-id = "org.mozilla.firefox"; };
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
          run = [ "move-node-to-workspace 2" ];
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
          "if" = { app-id = "notion.id"; };
          run = [ "move-node-to-workspace n" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.apple.iCal"; };
          run = [ "move-node-to-workspace e" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "ru.keepcoder.Telegram"; };
          run = [ "move-node-to-workspace 3" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.microsoft.Outlook"; };
          run = [ "move-node-to-workspace e" ];
        }
        {
          check-further-callbacks = false;
          "if" = { app-id = "com.apple.finder"; };
          run = [ "move-node-to-workspace w" ];
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
        "n" = "main";
        "d" = "main";
        "v" = "secondary";
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
        alt-n = "workspace n";
        alt-d = "workspace d";
        alt-v = "workspace v";
        alt-t = "workspace t";

        alt-shift-1 = [ "move-node-to-workspace 1" ];
        alt-shift-2 = [ "move-node-to-workspace 2" ];
        alt-shift-3 = [ "move-node-to-workspace 3" ];
        alt-shift-4 = [ "move-node-to-workspace 4" ];
        alt-shift-q = [ "move-node-to-workspace q" ];
        alt-shift-w = [ "move-node-to-workspace w" ];
        alt-shift-e = [ "move-node-to-workspace e" ];
        alt-shift-n = [ "move-node-to-workspace n" ];
        alt-shift-d = [ "move-node-to-workspace d" ];
        alt-shift-v = [ "move-node-to-workspace v" ];
        alt-shift-t = [ "move-node-to-workspace t" ];

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
