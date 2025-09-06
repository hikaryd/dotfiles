{ pkgs, ... }: {
  programs.tmux = {
    enable = true;
    terminal = "tmux-256color";
    prefix = "C-a";
    shell = "${pkgs.nushell}/bin/nu";
    escapeTime = 0;
    baseIndex = 1;
    mouse = true;
    keyMode = "vi";
    clock24 = true;
    historyLimit = 1000000;
    secureSocket = false;

    extraConfig = ''
      # Terminal and capabilities
      set -g default-terminal "tmux-256color"
      set -g xterm-keys on
      set -g extended-keys on
      # Force extended key reporting for terminals advertising as xterm-256color (e.g., Ghostty)
      set -as terminal-features 'xterm*:extkeys'

      set -ga terminal-overrides ",tmux-256color:Tc"
      set -as terminal-overrides ',*:sitm=\E[3m'
      set -as terminal-overrides ',*:Smulx=\E[4::%p1%dm'
      set -as terminal-overrides ',*:Setulc=\E[58::2::%p1%{65536}%/%d::%p1%{256}%/%{255}%&%d::%p1%{255}%&%d%;m'

      # Image support passthrough and activity
      set -gq allow-passthrough on
      set -g visual-activity off

      # Window handling and clipboard
      set -g renumber-windows on
      set -g set-clipboard on

      # Prefix and splits
      unbind C-b
      bind C-a send-prefix

      unbind %
      bind | split-window -h -c "#{pane_current_path}"

      unbind '"'
      bind _ split-window -v -c "#{pane_current_path}"
      unbind c
      bind c new-window -c "#{pane_current_path}"

      # Reload
      unbind r
      bind r source-file ~/.config/tmux/tmux.conf \; display "Reloaded!"

      # Focus navigation with C+hjkl (no prefix)
      bind-key -n C-h select-pane -L
      bind-key -n C-j select-pane -D
      bind-key -n C-k select-pane -U
      bind-key -n C-l select-pane -R

      # Resize with hjkl
      bind j resize-pane -D 5
      bind k resize-pane -U 5
      bind l resize-pane -R 5
      bind h resize-pane -L 5

      # Resize with Ctrl+Shift+hjkl (no prefix, repeatable)
      bind-key -n -r C-H resize-pane -L 5
      bind-key -n -r C-J resize-pane -D 5
      bind-key -n -r C-K resize-pane -U 5
      bind-key -n -r C-L resize-pane -R 5

      # Maximize pane
      bind -r m resize-pane -Z

      # Copy-mode (vi)
      bind v copy-mode -e \; send -X begin-selection
      bind-key -T copy-mode-vi 'v' send -X begin-selection
      bind-key -T copy-mode-vi 'y' send -X copy-selection

      # Move windows with Ctrl-Shift arrows
      # Use run-shell to avoid "no current target" at config load
      bind-key -n C-S-Left  run-shell 'tmux swap-window -d -t -1'
      bind-key -n C-S-Right run-shell 'tmux swap-window -d -t +1'

      # Killers
      bind w kill-window                 # prefix+w kills window
      bind-key -n C-w kill-pane          # C-w kills current pane

      # Navigation
      bind-key Space last-window
      bind b previous-window

      # Session switching
      unbind (
      unbind )
      unbind L
      bind-key [ switch-client -p
      bind-key ] switch-client -n
      bind-key Enter switch-client -l

      # tmux-sessionx options
      set -g @sessionx-bind-zo-new-window 'ctrl-y'
      set -g @sessionx-auto-accept 'off'
      set -g @sessionx-custom-paths '~/'
      # Bind sessionx to prefix+p
      set -g @sessionx-bind 'p'
      set -g @sessionx-x-path '~/dotfiles'
      set -g @sessionx-window-height '85%'
      set -g @sessionx-window-width '75%'
      set -g @sessionx-zoxide-mode 'on'
      set -g @sessionx-custom-paths-subdirectories 'false'
      set -g @sessionx-filter-current 'false'

      # Status and borders
      set -g status-position bottom
      set -g pane-active-border-style 'fg=magenta,bg=default'
      set -g pane-border-style 'fg=brightblack,bg=default'

      set -g status-bg "#24273a"
      set -g status-left-length 0
      set -g status-left ""
      set -ga status-left "#{?client_prefix,#[fg=#{@thm_bg} bg=#{@thm_red} bold]   #[fg=#{@thm_red} bg=#{@thm_surface_2}],#[fg=#{@thm_bg} bg=#{@thm_lavender} bold]   #[fg=#{@thm_lavender} bg=#{@thm_surface_2}]}#[fg=#{@thm_surface_2} bg=#{@thm_surface_1}]#[fg=#{@thm_surface_1} bg=#{@thm_bg}]"
      set -g status-right "#[fg=#{@thm_surface_1},bg=#{@thm_bg}]#[fg=#{@thm_lavender},bg=#{@thm_surface_1}]#[fg=#{@thm_surface_2},bg=#{@thm_surface_1}]#[fg=#{@thm_lavender},bg=#{@thm_surface_2}]#[fg=#{@thm_lavender},bg=#{@thm_surface_2}]#[fg=#{@thm_bg},bg=#{@thm_lavender},bold] #S "
    '';

    plugins = with pkgs; [
      tmuxPlugins.vim-tmux-navigator
      tmuxPlugins.tmux-fzf
      {
        plugin = tmuxPlugins.tmux-sessionx;
        extraConfig = ''
          set -g @sessionx-bind 'p'
        '';
      }

      {
        plugin = tmuxPlugins.resurrect;
        extraConfig = ''
          set -g @resurrect-capture-pane-contents 'on'
        '';
      }

      {
        plugin = tmuxPlugins.continuum;
        extraConfig = "set -g @continuum-restore 'on'";
      }

      {
        plugin = tmuxPlugins.catppuccin;
        extraConfig = ''
          set -g @catppuccin_flavor 'macchiato'
          set -g @catppuccin_window_number_position "left"
          set -g @catppuccin_window_current_text_color "#{@thm_peach}"
          set -g @catppuccin_window_current_number_color "#{@thm_surface_0}"
          set -g @catppuccin_window_number_color "#{@thm_bg}"
          set -g @catppuccin_window_text_color "#{@thm_bg}"
          set -g @catppuccin_window_text "#[fg=#{@thm_fg}]#W "
          set -g @catppuccin_window_number "#[fg=#{@thm_fg}] #I"
          set -g @catppuccin_window_current_text "#[fg=#{@thm_bg} bold] #W#{?window_zoomed_flag,(),} "
          set -g @catppuccin_window_current_number "#[bg=#{@thm_surface_0},fg=#{@thm_peach} bold] #I "
          set -g @catppuccin_window_left_separator ""
          set -g @catppuccin_window_right_separator ""
          set -g @catppuccin_window_status_style "custom"
          set -g @catppuccin_window_current_left_separator "#[bg=#{@catppuccin_window_current_number_color},fg=#{@thm_bg}]"
          set -g @catppuccin_window_current_right_separator "#[bg=#{@thm_bg},fg=#{@catppuccin_window_current_text_color}]"
          set -g @catppuccin_window_middle_separator "  "
          set -g @catppuccin_window_current_middle_separator "#[bg=#{@catppuccin_window_current_text_color},fg=#{@catppuccin_window_current_number_color}]"

          # Ensure our pane navigation binds override plugin defaults
          unbind -n C-h
          unbind -n C-j
          unbind -n C-k
          unbind -n C-l
          bind-key -n C-h select-pane -L
          bind-key -n C-j select-pane -D
          bind-key -n C-k select-pane -U
          bind-key -n C-l select-pane -R

          # Ctrl+1..9 to switch windows without prefix (requires extended-keys)
          bind-key -n C-1 select-window -t 1
          bind-key -n C-2 select-window -t 2
          bind-key -n C-3 select-window -t 3
          bind-key -n C-4 select-window -t 4
          bind-key -n C-5 select-window -t 5
          bind-key -n C-6 select-window -t 6
          bind-key -n C-7 select-window -t 7
          bind-key -n C-8 select-window -t 8
          bind-key -n C-9 select-window -t 9

          # Keep pane backgrounds transparent: use terminal default bg
          # This prevents tmux (and the theme) from forcing an opaque pane bg,
          # so Ghostty's background opacity still applies inside tmux.
          set -g window-style 'bg=default'
          set -g window-active-style 'bg=default'
        '';
      }
    ];
  };
}
