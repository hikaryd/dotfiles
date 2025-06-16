{ pkgs, ... }: {
  programs.tmux = {
    enable = false;
    terminal = "xterm-256color";
    prefix = "C-a";
    shell = "${pkgs.nushell}/bin/nu";
    escapeTime = 1;
    baseIndex = 1;
    customPaneNavigationAndResize = true;
    clock24 = true;
    historyLimit = 1000000;
    secureSocket = false;
    extraConfig = ''
      set -ag terminal-overrides ",xterm-256color:RGB"
      set -g window-status-style bg=default
      set -g window-status-current-style bg=default
      set -g status-style bg=default
      set -g status-style bg=default
      bind-key "T" run-shell "sesh connect \"$(
        sesh list --icons | fzf-tmux -p 80%,70% \
          --no-sort --ansi --border-label ' sesh ' --prompt '⚡  ' \
          --header '  ^a all ^t tmux ^g configs ^x zoxide ^d tmux kill ^f find' \
          --bind 'tab:down,btab:up' \
          --bind 'ctrl-a:change-prompt(⚡  )+reload(sesh list --icons)' \
          --bind 'ctrl-t:change-prompt(🪟  )+reload(sesh list -t --icons)' \
          --bind 'ctrl-g:change-prompt(⚙️  )+reload(sesh list -c --icons)' \
          --bind 'ctrl-x:change-prompt(📁  )+reload(sesh list -z --icons)' \
          --bind 'ctrl-f:change-prompt(🔎  )+reload(fd -H -d 2 -t d -E .Trash . ~)' \
          --bind 'ctrl-d:execute(tmux kill-session -t {2..})+change-prompt(⚡  )+reload(sesh list --icons)' \
          --preview-window 'right:55%' \
          --preview 'sesh preview {}'
      )\""

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

      bind : command-prompt
      bind * setw synchronize-panes
      bind P set pane-border-status
      bind c kill-pane
      bind x swap-pane -D
      bind S choose-session
      bind K send-keys "clear"\; send-keys "Enter"
      bind-key -T copy-mode-vi v send-keys -X begin-selection
      bind-key m command-prompt -p "Переместить окно на позицию: " "run-shell '~/.config/tmux/scripts/move_windows.sh %%'"
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

      bind-key -T copy-mode-vi 'C-h' select-pane -L
      bind-key -T copy-mode-vi 'C-j' select-pane -D
      bind-key -T copy-mode-vi 'C-k' select-pane -U
      bind-key -T copy-mode-vi 'C-l' select-pane -R

      bind-key -n C-h if -F "#{@pane-is-vim}" 'send-keys C-h'  'select-pane -L'
      bind-key -n C-j if -F "#{@pane-is-vim}" 'send-keys C-j'  'select-pane -D'
      bind-key -n C-k if -F "#{@pane-is-vim}" 'send-keys C-k'  'select-pane -U'
      bind-key -n C-l if -F "#{@pane-is-vim}" 'send-keys C-l'  'select-pane -R'

      bind-key -n C-H if -F "#{@pane-is-vim}" 'send-keys M-h' 'resize-pane -L 3'
      bind-key -n C-J if -F "#{@pane-is-vim}" 'send-keys M-j' 'resize-pane -D 3'
      bind-key -n C-K if -F "#{@pane-is-vim}" 'send-keys M-k' 'resize-pane -U 3'
      bind-key -n C-L if -F "#{@pane-is-vim}" 'send-keys M-l' 'resize-pane -R 3'

      bind-key -n M-H swap-window -t -1\; select-window -t -1
      bind-key -n M-L swap-window -t +1\; select-window -t +1
      bind h select-pane -L
      bind j select-pane -D
      bind k select-pane -U
      bind l select-pane -R
    '';

    plugins = with pkgs; [
      tmuxPlugins.yank
      tmuxPlugins.tmux-thumbs
      {
        plugin = pkgs.tmuxPlugins.resurrect;
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
    ];
  };

  xdg.configFile = {
    "tmux/scripts/move_windows.sh" = {
      executable = true;
      text = # bash
        ''
          #!/bin/bash
          if [ "$#" -ne 1 ]; then
            echo "Usage: ''${0} <new_position>"
            exit 1
          fi
          NEW_POSITION=$1
          CURRENT_WINDOW=$(tmux display-message -p '#I')
          if tmux list-windows -F '#I' | grep -q "^''${NEW_POSITION}$"; then
            TEMP_INDEX=100
            while tmux list-windows -F '#I' | grep -q "^''${TEMP_INDEX}$"; do
              TEMP_INDEX=$((TEMP_INDEX + 1))
            done
            tmux move-window -s ''${CURRENT_WINDOW} -t ''${TEMP_INDEX}
            if [ ''${NEW_POSITION} -lt ''${CURRENT_WINDOW} ]; then
              for ((i = ''${CURRENT_WINDOW} - 1; i >= ''${NEW_POSITION}; i--)); do
                tmux move-window -s $i -t $((i + 1))
              done
            else
              for ((i = ''${CURRENT_WINDOW} + 1; i <= ''${NEW_POSITION}; i++)); do
                tmux move-window -s $i -t $((i - 1))
              done
            fi
            tmux move-window -s ''${TEMP_INDEX} -t ''${NEW_POSITION}
          else
            tmux move-window -s ''${CURRENT_WINDOW} -t ''${NEW_POSITION}
          fi
        '';
    };
  };
}
