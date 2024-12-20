{ pkgs, ... }: {
  home.packages = with pkgs; [
    tmux
    tmuxPlugins.sensible
    tmuxPlugins.yank
    tmuxPlugins.resurrect
    tmuxPlugins.continuum
    tmuxPlugins.tmux-thumbs
    tmuxPlugins.tmux-fzf
    tmuxPlugins.catppuccin
    calcurse
  ];

  xdg.configFile = {
    "tmux/scripts/move_windows.sh" = {
      executable = true;
      text = ''
        #!/usr/bin/env bash

        target="$1"
        current=$(tmux display-message -p '#I')

        if [[ "$target" =~ ^[0-9]+$ ]]; then
            if [ "$target" -eq "$current" ]; then
                exit 0
            fi
            
            windows=($(tmux list-windows -F '#I'))
            
            if [[ " ''${windows[@]} " =~ " $target " ]]; then
                tmp=$((target + 1))
                while [[ " ''${windows[@]} " =~ " $tmp " ]]; do
                    ((tmp++))
                done
                tmux move-window -t $tmp
                tmux move-window -s $tmp -t $target
                tmux move-window -t $target
            else
                tmux move-window -t $target
            fi
        fi
      '';
    };

    "tmux/scripts/cal.sh" = {
      executable = true;
      text = ''
        #!/usr/bin/env bash

        DATE_REGEX="[0-9]{4}-[0-9]{2}-[0-9]{2}"
        TIME_REGEX="[0-9]{2}:[0-9]{2}"
        DATETIME_REGEX="''${DATE_REGEX} ''${TIME_REGEX}"

        get_calendar_app() {
            if command -v "${pkgs.calcurse}/bin/calcurse" >/dev/null 2>&1; then
                echo "${pkgs.calcurse}/bin/calcurse"
            else
                echo ""
            fi
        }

        get_next_meeting() {
            local cal_app
            cal_app=$(get_calendar_app)

            if [ -n "$cal_app" ]; then
                $cal_app -n | \
                    grep -m1 "$DATETIME_REGEX" | \
                    sed -E "s/^[^0-9]*($DATETIME_REGEX)/$1/"
            fi
        }

        main() {
            local next_meeting
            next_meeting=$(get_next_meeting)

            if [ -n "$next_meeting" ]; then
                echo "$next_meeting"
            fi
        }

        main
      '';
    };

    "tmux/tmux.reset.conf" = {
      text = ''
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
        bind R source-file ~/.config/tmux/tmux.conf
        bind K send-keys "clear"\; send-keys "Enter"
        bind-key -T copy-mode-vi v send-keys -X begin-selection
        bind-key M-m command-prompt -p "Переместить окно на позицию: " "run-shell '~/.config/tmux/scripts/move_windows.sh %%'"
        bind-key S run-shell ~/.config/tmux/plugins/tmux-resurrect/scripts/save.sh
      '';
    };
  };

  programs.tmux = {
    enable = true;
    baseIndex = 1;
    escapeTime = 0;
    historyLimit = 1000000;
    keyMode = "vi";
    mouse = true;
    prefix = "C-a";
    terminal = "xterm-256color";
    shell = "${pkgs.fish}/bin/fish";

    extraConfig = ''
      source-file ~/.config/tmux/tmux.reset.conf
      set-option -g terminal-overrides ',xterm-256color:RGB'

      set -g detach-on-destroy off     # don't exit from tmux when closing a session
      set -g renumber-windows on       # renumber all windows when any window is closed
      set -g set-clipboard on          # use system clipboard
      set -g status-position top       # macOS / darwin style

      set -g pane-active-border-style 'fg=magenta,bg=default'
      set -g pane-border-style 'fg=brightblack,bg=default'

      # Plugin configurations
      set -g @continuum-restore 'on'
      set -g @resurrect-strategy-nvim 'session'

      # FZF configurations
      set -g @fzf-url-fzf-options '-p 60%,30% --prompt="   " --border-label=" Open URL "'
      set -g @fzf-url-history-limit '2000'
    '';

    plugins = with pkgs.tmuxPlugins; [
      sensible
      yank
      resurrect
      continuum
      tmux-thumbs
      tmux-fzf
      {
        plugin = catppuccin;
        extraConfig = ''
          # tmux-prefix-highlight
          set -g @catppuccin_window_left_separator ""
          set -g @catppuccin_window_right_separator " "
          set -g @catppuccin_window_middle_separator " █"
          set -g @catppuccin_window_number_position "right"
          set -g @catppuccin_window_default_fill "number"
          set -g @catppuccin_window_default_text "#W"
          set -g @catppuccin_window_current_fill "number"
          set -g @catppuccin_window_current_text "#W#{?window_zoomed_flag,(),}"
          set -g @catppuccin_status_modules_right "directory date_time"
          set -g @catppuccin_status_modules_left "session"
          set -g @catppuccin_status_left_separator  " "
          set -g @catppuccin_status_right_separator " "
          set -g @catppuccin_status_right_separator_inverse "no"
          set -g @catppuccin_status_fill "icon"
          set -g @catppuccin_status_connect_separator "no"
          set -g @catppuccin_directory_text "#{b:pane_current_path}"
          set -g @catppuccin_meetings_text "#($HOME/.config/tmux/scripts/cal.sh)"
          set -g @catppuccin_date_time_text "%H:%M"
        '';
      }
    ];
  };
}
