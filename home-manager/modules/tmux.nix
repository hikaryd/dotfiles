{ pkgs, ... }: {
  home.packages = with pkgs; [ sesh gum ];
  programs.tmux = {
    enable = true;
    terminal = "tmux-256color";
    prefix = "C-a";
    shell = "${pkgs.nushell}/bin/nu";
    escapeTime = 0;
    baseIndex = 1;
    keyMode = "vi";
    mouse = true;
    historyLimit = 1000000;
    secureSocket = false;

    plugins = [
      pkgs.tmuxPlugins.sensible
      {
        plugin =
          (pkgs.tmuxPlugins.tokyo-night-tmux or pkgs.tmuxPlugins.catppuccin);
        extraConfig = ''
          set -g @tokyo-night-tmux_theme "rose-pine"
          set -g @tokyo-night-tmux_show_datetime 0
          set -g @tokyo-night-tmux_show_path 1
          set -g @tokyo-night-tmux_path_format relative
          set -g @tokyo-night-tmux_window_id_style dsquare
          set -g @tokyo-night-tmux_show_git 0
        '';
      }
      {
        plugin = pkgs.tmuxPlugins.resurrect;
        extraConfig = ''
          set -g @resurrect-dir "$HOME/.local/share/tmux/resurrect"
          set -g @resurrect-capture-pane-contents 'on'
          set -g @resurrect-strategy-nvim 'session'
          set -g @resurrect-strategy-vim  'session'
        '';
      }
      {
        plugin = pkgs.tmuxPlugins.continuum;
        extraConfig = ''
          # авто-сейв и авто-восстановление последнего снапшота
          set -g @continuum-restore 'on'
          # как часто сохранять (в минутах)
          set -g @continuum-save-interval '15'
        '';
      }
    ];

    extraConfig = ''
      ################################################################################
      set -as terminal-overrides ',*256col*:RGB'
      set -as terminal-overrides ',*:Smulx=\E[4::%p1%dm'  # undercurl
      set -as terminal-overrides ',*:Setulc=\E[58::2::%p1%{65536}%/%d::%p1%{256}%/%{255}%&%d::%p1%{255}%&%d%;m' # underscore colors

      set -g xterm-keys on
      set -g extended-keys on
      set -as terminal-features 'xterm*:extkeys'

      ################################################################################
      unbind C-b
      set -g pane-base-index 1
      set -g detach-on-destroy off
      set -g focus-events on
      set -g renumber-windows on
      set -g status-position top

      ################################################################################
      set -g prefix C-a
      bind C-a send-prefix
      bind r source-file ~/.config/tmux/tmux.conf \; display-message "Reloaded!"

      unbind %
      bind | split-window -h -c "#{pane_current_path}"
      unbind '"'
      bind _ split-window -v -c "#{pane_current_path}"
      unbind c
      bind c new-window -c "#{pane_current_path}"

      bind-key -n C-h select-pane -L
      bind-key -n C-j select-pane -D
      bind-key -n C-k select-pane -U
      bind-key -n C-l select-pane -R

      bind h resize-pane -L 5
      bind j resize-pane -D 5
      bind k resize-pane -U 5
      bind l resize-pane -R 5

      bind-key -n -r C-H resize-pane -L 5
      bind-key -n -r C-J resize-pane -D 5
      bind-key -n -r C-K resize-pane -U 5
      bind-key -n -r C-L resize-pane -R 5

      bind -r m resize-pane -Z

      bind v copy-mode -e \; send -X begin-selection
      bind-key -T copy-mode-vi 'v' send -X begin-selection
      bind-key -T copy-mode-vi 'y' send -X copy-selection

      bind-key -n C-S-Left  run-shell 'tmux swap-window -d -t -1'
      bind-key -n C-S-Right run-shell 'tmux swap-window -d -t +1'

      bind w kill-window
      bind-key -n C-w kill-pane

      bind-key Space last-window
      bind b previous-window

      unbind (
      unbind )
      unbind L
      bind-key [ switch-client -p
      bind-key ] switch-client -n
      bind-key Enter switch-client -l

      bind-key -n C-1 select-window -t 1
      bind-key -n C-2 select-window -t 2
      bind-key -n C-3 select-window -t 3
      bind-key -n C-4 select-window -t 4
      bind-key -n C-5 select-window -t 5
      bind-key -n C-6 select-window -t 6
      bind-key -n C-7 select-window -t 7
      bind-key -n C-8 select-window -t 8
      bind-key -n C-9 select-window -t 9

      set -g window-style 'bg=default'
      set -g window-active-style 'bg=default'

      bind g popup -EE -w 90% -h 90% -d "#{pane_current_path}" lazygit
      bind-key T display-popup -E -w 30% -h 30% 'bash -lc "
      set -euo pipefail
      sel=$(sesh list -i \
        | gum filter --limit 1 --no-sort --fuzzy \
                     --placeholder \"Pick a sesh\" --height 50 --prompt=\" \")
      [ -z \"$sel\" ] && exit 0
      sel=$(printf \"%s\" \"$sel\" | tr -d \"\r\" | sed -E 's/[[:space:]]+$//')
      exec sesh connect \"$sel\"
      "'
      bind -N "last-session (via sesh)" L run-shell "sesh last"

      set -g default-shell "${pkgs.nushell}/bin/nu"
      set -g default-command "${pkgs.nushell}/bin/nu"
      set-environment -g SHELL "${pkgs.nushell}/bin/nu"
    '';
  };
}
