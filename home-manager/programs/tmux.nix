{ pkgs, ... }: {
  programs.tmux = {
    enable = true;
    shell = "${pkgs.zsh}/bin/zsh";
    terminal = "screen-256color";
    prefix = "C-a";
    baseIndex = 1;
    escapeTime = 0;
    historyLimit = 1000000;
    keyMode = "vi";
    secureSocket = false;
    extraConfig = # bash
      ''
        set -g status-style bg=default

        set -g window-status-current-format "#[fg=#a6da95] #I #W "
        set -g window-status-format "#[fg=#8aadf4] #I #W "

        set -g status-left "#[fg=#8aadf4]#S "
        set -g status-left-length 50

        set -g status-right " #[fg=#8aadf4]%H:%M "

        set -g status-justify centre

        set-option -g terminal-overrides ',xterm-256color:RGB'

        set -g detach-on-destroy off
        set -g renumber-windows on
        set -g set-clipboard on
        set -g status-position top
        set -g pane-active-border-style 'fg=magenta,bg=default'
        set -g pane-border-style 'fg=brightblack,bg=default'

        bind ^X lock-server
        bind ^C new-window -c "$HOME"
        bind ^D detach
        bind * list-clients

        bind H previous-window
        bind L next-window

        bind R source-file ~/.config/tmux/tmux.conf
        bind ^A last-window
        bind ^W list-windows
        bind w list-windows
        bind z resize-pane -Z
        bind ^L refresh-client
        bind l refresh-client
        bind | split-window
        bind s split-window -v -c "#{pane_current_path}"
        bind v split-window -h -c "#{pane_current_path}"
        bind '"' choose-window
        bind h select-pane -L
        bind j select-pane -D
        bind k select-pane -U
        bind l select-pane -R
        bind -r -T prefix - resize-pane -L 20
        bind -r -T prefix = resize-pane -R 20
        bind -r -T prefix _ resize-pane -D 7
        bind -r -T prefix + resize-pane -U 7
        bind : command-prompt
        bind * setw synchronize-panes
        bind P set pane-border-status
        bind c kill-pane
        bind x swap-pane -D
        bind S choose-session
        bind K send-keys "clear"\; send-keys "Enter"
        bind-key -T copy-mode-vi v send-keys -X begin-selection
        bind-key M-m command-prompt -p "Переместить окно на позицию: " "run-shell '~/.config/tmux/scripts/move_windows.sh %%'"
        bind-key S run-shell ~/.config/tmux/plugins/tmux-resurrect/scripts/save.sh
        bind R source-file ~/.config/tmux/tmux.conf
        bind ^A last-window
        bind ^W list-windows
        bind w list-windows
        bind z resize-pane -Z
        bind ^L refresh-client
        bind l refresh-client
        bind | split-window
        bind s split-window -v -c "#{pane_current_path}"
        bind v split-window -h -c "#{pane_current_path}"

        bind-key -n M-H swap-window -t -1\; select-window -t -1
        bind-key -n M-L swap-window -t +1\; select-window -t +1
        bind h select-pane -L
        bind j select-pane -D
        bind k select-pane -U
        bind l select-pane -R
      '';

    plugins = with pkgs; [
      tmuxPlugins.sensible
      tmuxPlugins.yank
      tmuxPlugins.resurrect
      tmuxPlugins.continuum
      tmuxPlugins.tmux-fzf
      tmuxPlugins.tmux-floax
      tmuxPlugins.tmux-thumbs
      {
        plugin = tmuxPlugins.resurrect;
        extraConfig = "set -g @resurrect-strategy-nvim 'session'";
      }
      {
        plugin = tmuxPlugins.continuum;
        extraConfig = "set -g @continuum-restore 'on'";
      }
      {
        plugin = tmuxPlugins.tmux-fzf;
        extraConfig = ''
          set -g @fzf-url-fzf-options '-p 60%,30% --prompt="   " --border-label=" Open URL "'
          set -g @fzf-url-history-limit '2000'
        '';
      }
      {
        plugin = tmuxPlugins.tmux-floax;
        extraConfig = ''
          set -g @floax-width '80%'
          set -g @floax-height '80%'
          set -g @floax-border-color 'magenta'
          set -g @floax-text-color 'blue'
          set -g @floax-bind 'p'
          set -g @floax-change-path 'true'
        '';
      }
      {
        plugin = (pkgs.tmuxPlugins.mkTmuxPlugin {
          pname = "tmux-sessionx";
          pluginName = "tmux-sessionx";
          version = "1.0.0";
          src = pkgs.fetchFromGitHub {
            owner = "omerxx";
            repo = "tmux-sessionx";
            rev = "c2eb0e19bf3ffba2b64e1ab63cdf37cb61f53e3c";
            sha256 = "sha256-5f2lADOgCSSfFrPy9uiTomtjSZPkEAMEDu4/TdDYXlk=";
          };
          postInstall = ''
            mv $target/sessionx.tmux $target/tmux_sessionx.tmux
          '';
        });
        extraConfig = ''
          set -g @sessionx-bind-zo-new-window 'ctrl-y'
          set -g @sessionx-auto-accept 'off'
          set -g @sessionx-bind 'o'
          set -g @sessionx-window-height '85%'
          set -g @sessionx-window-width '75%'
          set -g @sessionx-zoxide-mode 'on'
          set -g @sessionx-filter-current 'false'
        '';
      }
    ];
  };

  xdg.configFile = {
    "tmux/scripts/move_windows.sh" = {
      executable = true;
      text = ''
        #!/usr/bin/env bash
        tmux move-window -t "$1"
      '';
    };
  };
}
