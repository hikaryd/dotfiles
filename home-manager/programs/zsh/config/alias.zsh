#!/bin/zsh

alias v='nvim'
alias proxy='mgraftcp --socks5 127.0.0.1:2080'
alias kvnt_db_test='rainfrog --url $(cat ~/creds/kvant_test_db_url)'
alias kvnt_db_email='rainfrog --url $(cat ~/creds/kvant_email_db_url)'
alias lg='lazygit'
alias ta='tmux attach'
alias ..='cd ..'
alias ld='oxker'
alias s='sudo'
alias cd='z'
alias music='mgraftcp --socks5 127.0.0.1:2080 spotify_player'
alias vs='source .venv/bin/activate'
alias l='eza -lah --icons=auto --hyperlink'
alias tree='eza --tree --level=2 --icons'
alias csv-view='csvlens'
alias parse_dir="repomix --ignore '*.lock,docs/*,.git/*,.idea/*,.vscode/*,__pycache__'"
alias ssh="ssh -A"
alias kvnt_ssh_agent='eval $(ssh-agent) && ssh-add ~/.ssh/kvant-test-app'
alias nix_clear='
  nix-collect-garbage -d && \
  nix-channel --add https://nixos.org/channels/nixpkgs-unstable nixpkgs && \
  nix-channel --add https://github.com/nix-community/home-manager/archive/master.tar.gz home-manager && \
  nix-channel --add https://github.com/catppuccin/nix/archive/main.tar.gz catppuccin && \
  nix-channel --update && \
  nix-shell '"'"'<home-manager>'"'"' -A install
'
