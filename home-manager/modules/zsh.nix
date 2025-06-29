{ pkgs, ... }: {
  programs.zsh = {
    enable = true;
    autocd = true;
    enableCompletion = true;
    autosuggestion.enable = true;
    syntaxHighlighting.enable = true;

    history = {
      ignoreDups = true;
      ignoreAllDups = true;
      ignoreSpace = true;
      save = 1000000;
      size = 1000000;
      path = "$HOME/.zsh_history";
    };

    shellAliases = {
      v = "nvim";
      l = "eza -l --icons --git";
      la = "eza -la --icons --git";
      tree = "eza --tree --icons";

      ".." = "cd ..";
      "..." = "cd ../..";
      cd = "z";
      c = "clear";

      lg = "lazygit";

      hms = "sudo darwin-rebuild switch --flake '.#hikary' --impure";
      hmc = "nix-collect-garbage -d";
      src = "exec zsh";

      fdev = "cd $HOME/Documents/dev";
    };

    initContent = ''
      # Профилирование
      # zmodload zsh/zprof

      # Homebrew
      export PATH="/opt/homebrew/bin:/opt/homebrew/sbin:$PATH"
      export HOMEBREW_PREFIX="/opt/homebrew"
      export HOMEBREW_CELLAR="/opt/homebrew/Cellar"
      export HOMEBREW_REPOSITORY="/opt/homebrew"

      autoload -Uz compinit
      if [ "$(date +'%j')" != "$(stat -f '%Sm' -t '%j' ~/.zcompdump 2>/dev/null)" ]; then
        compinit
      else
        compinit -C
      fi

      setopt AUTO_CD
      setopt APPEND_HISTORY
      setopt SHARE_HISTORY  
      setopt HIST_IGNORE_SPACE
      setopt HIST_IGNORE_ALL_DUPS
      setopt HIST_SAVE_NO_DUPS
      setopt HIST_FIND_NO_DUPS
      setopt INC_APPEND_HISTORY

      # Keybindings
      bindkey -e
      bindkey '^p' history-search-backward
      bindkey '^n' history-search-forward
      bindkey "^[[A" history-beginning-search-backward
      bindkey "^[[B" history-beginning-search-forward

      # Completion стили (красивые)
      zstyle ':completion:*' matcher-list 'm:{a-z}={A-Za-z}'
      zstyle ':completion:*' menu select
      zstyle ':completion:*' list-colors ''${(s.:.)LS_COLORS}
      zstyle ':completion:*' group-name ""
      zstyle ':completion:*:descriptions' format '%F{yellow}%B-- %d --%b%f'
      zstyle ':completion:*:messages' format '%F{purple}-- %d --%f'
      zstyle ':completion:*:warnings' format '%F{red}-- no matches found --%f'
      zstyle ':completion:*' use-cache on
      zstyle ':completion:*' cache-path ~/.zsh/cache

      # FZF настройки (Catppuccin цвета)
      export FZF_DEFAULT_OPTS="--height 40% --layout=reverse --border --color=fg:#cdd6f4,bg:#1e1e2e,hl:#f38ba8,fg+:#cdd6f4,bg+:#313244,hl+:#f38ba8,info:#cba6f7,prompt:#cba6f7,pointer:#f5e0dc,marker:#f5e0dc,spinner:#f5e0dc,header:#94e2d5"
      export FZF_CTRL_T_OPTS="--preview 'bat --color=always --style=numbers --line-range=:500 {}'"
      export FZF_CTRL_R_OPTS="--preview 'echo {}' --preview-window down:3:hidden:wrap --bind '?:toggle-preview'"
      export FZF_ALT_C_OPTS="--preview 'eza --tree --color=always {} | head -200'"

      # Shell integrations
      eval "$(fzf --zsh)"
      eval "$(zoxide init --cmd cd zsh)"

      # FZF-TAB для красивых completion (lazy load для скорости)
      _load_fzf_tab() {
        if [[ ! -d ~/.zsh/fzf-tab ]]; then
          git clone --depth 1 https://github.com/Aloxaf/fzf-tab ~/.zsh/fzf-tab
        fi
        source ~/.zsh/fzf-tab/fzf-tab.plugin.zsh
        
        # FZF-TAB настройки
        zstyle ':fzf-tab:complete:cd:*' fzf-preview 'eza -1 --color=always $realpath'
        zstyle ':fzf-tab:complete:__zoxide_z:*' fzf-preview 'eza -1 --color=always $realpath'
        zstyle ':fzf-tab:complete:ls:*' fzf-preview 'eza -1 --color=always $realpath'
        zstyle ':fzf-tab:complete:eza:*' fzf-preview 'eza -1 --color=always $realpath'
        zstyle ':fzf-tab:complete:nvim:*' fzf-preview 'bat --color=always --style=numbers --line-range=:500 $realpath'
        zstyle ':fzf-tab:complete:cat:*' fzf-preview 'bat --color=always --style=numbers --line-range=:500 $realpath'
        zstyle ':fzf-tab:complete:git-(add|diff|restore):*' fzf-preview 'git diff --color=always $word'
        zstyle ':fzf-tab:complete:git-log:*' fzf-preview 'git log --color=always $word'
        zstyle ':fzf-tab:complete:git-show:*' fzf-preview 'git show --color=always $word'
        zstyle ':fzf-tab:complete:git-checkout:*' fzf-preview 'git log --color=always $word'
        zstyle ':fzf-tab:complete:kill:argument-rest' fzf-preview 'ps --pid=$word -o cmd --no-headers -w -w'
        zstyle ':fzf-tab:complete:kill:argument-rest' fzf-flags '--preview-window=down:3:wrap'
        
        # UX настройки
        zstyle ':completion:complete:*:options' sort false
        zstyle ':fzf-tab:*' switch-group ',' '.'
        zstyle ':fzf-tab:*' fzf-command fzf
      }

      # Загружаем fzf-tab при первом использовании TAB
      _original_complete_word=$(bindkey '^I' | cut -d' ' -f2)
      _smart_tab() {
        if [[ ! -f ~/.zsh/fzf-tab-loaded ]]; then
          _load_fzf_tab
          touch ~/.zsh/fzf-tab-loaded
        fi
        zle $_original_complete_word
      }
      zle -N _smart_tab
      bindkey '^I' _smart_tab

      if [[ $TERM_PROGRAM == "WarpTerminal" ]]; then
        export PROMPT=""
      else
        export PROMPT='%F{blue}%B%~%b%f %F{green}❯%f '
      fi

      # zprof
    '';
  };

  home.packages = with pkgs; [
    eza # современный ls
    fzf # fuzzy finder
    zoxide # smart cd
    bat # cat с подсветкой для preview
    tree # для preview директорий
  ];
}
