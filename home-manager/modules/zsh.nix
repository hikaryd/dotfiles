{ pkgs, ... }: {
  programs.zsh = {
    enable = true;
    autocd = true;
    autosuggestion = { enable = true; };

    history = {
      ignoreDups = true;
      ignoreAllDups = true;
      ignoreSpace = true;
      save = 1000000;
      size = 1000000;
    };

    shellAliases = {
      src = "source ~/.zshrc";
      v = "nvim";
      home = "cd ~";
      lss = "yazi";
      vs = "source .venv/bin/activate";
      la = "eza -la --color=always --icons --group-directories-first";
      l = "eza -l --color=always --icons --group-directories-first";
      create_mr = "${../../scripts/ai_helper} --mode mr --api gemini";
      dots = "chezmoi";
      fdev = "cd $HOME/Documents/dev";
      cd = "z";
      net = "gping www.google.com -c '#88C0D0,#B48EAD,#81A1C1,#8FBCBB'";
      tree = "tre";
      fzf = "fzf --color=16 --border=rounded --margin=1,3 --preview='bat {}'";
      speedtest = "networkquality";
      zf =
        "nvim $(fzf --color=16 --border=rounded --margin=1,3 --preview='bat {}')";
      "." = "cd ..";
      ".." = "cd ../..";
      "..." = "cd ../../..";
      hms = "sudo darwin-rebuild switch --flake '.#hikary' --impure -v";
      hmc = "nix-collect-garbage -d";
      lg = "lazygit";
    };

    enableCompletion = true;
    antidote.enable = false;

    initContent = ''
      # Homebrew для macOS
      if [[ -f "/opt/homebrew/bin/brew" ]] then
        eval "$(/opt/homebrew/bin/brew shellenv)"
      fi

      # Zinit setup
      ZINIT_HOME="''${XDG_DATA_HOME:-''${HOME}/.local/share}/zinit/zinit.git"

      # Download Zinit, if it's not there yet
      if [ ! -d "$ZINIT_HOME" ]; then
         mkdir -p "$(dirname $ZINIT_HOME)"
         git clone https://github.com/zdharma-continuum/zinit.git "$ZINIT_HOME"
      fi

      # Source/Load zinit
      source "''${ZINIT_HOME}/zinit.zsh"

      zinit ice depth=1

      # Add in zsh plugins
      zinit light zsh-users/zsh-syntax-highlighting
      zinit light zsh-users/zsh-completions
      zinit light zsh-users/zsh-autosuggestions
      zinit light Aloxaf/fzf-tab

      # Add in snippets
      zinit snippet OMZL::git.zsh
      zinit snippet OMZP::git
      zinit snippet OMZP::sudo
      zinit snippet OMZP::aws
      zinit snippet OMZP::kubectl
      zinit snippet OMZP::kubectx
      zinit snippet OMZP::command-not-found

      # Load completions
      autoload -Uz compinit && compinit

      zinit cdreplay -q

      # Keybindings
      bindkey -e
      bindkey '^p' history-search-backward
      bindkey '^n' history-search-forward
      bindkey '^[w' kill-region

      # Completion styling
      zstyle ':completion:*' matcher-list 'm:{a-z}={A-Za-z}'
      zstyle ':completion:*' list-colors "''${(s.:.)LS_COLORS}"
      zstyle ':completion:*' menu no
      zstyle ':fzf-tab:complete:cd:*' fzf-preview 'ls --color $realpath'
      zstyle ':fzf-tab:complete:__zoxide_z:*' fzf-preview 'ls --color $realpath'

      # Shell integrations
      eval "$(fzf --zsh)"
      eval "$(zoxide init --cmd cd zsh)"
      setopt appendhistory
      setopt sharehistory
      setopt hist_ignore_space
      setopt hist_ignore_all_dups
      setopt hist_save_no_dups
      setopt hist_ignore_dups
      setopt hist_find_no_dups
    '';
  };

  home.packages = with pkgs; [ eza ];
}
