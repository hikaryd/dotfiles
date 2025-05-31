{
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
      l = "eza --color=always --icons --group-directories-first";
      la = "eza -a --color=always --icons --group-directories-first";
      ll = "eza -l --color=always --icons --group-directories-first";
      rm = "rm_confirm";
      create_mr = "${../../scripts/ai_helper} --mode mr --api gemini";
      dots = "chezmoi";
      fdev = "cd $HOME/Documents/dev";
      cd = "z";
      net = "gping www.google.com -c '#88C0D0,#B48EAD,#81A1C1,#8FBCBB'";
      tree = "tre";
      fzf = "fzf --color=16 --border=rounded --margin=1,3 --preview='bat {}'";
      zf =
        "nvim $(fzf --color=16 --border=rounded --margin=1,3 --preview='bat {}')";
      "." = "cd ..";
      ".." = "cd ../..";
      "..." = "cd ../../..";

      hms = "nix run nix-darwin -- switch --flake '.#hikary' --impure -v";
      hmc = "nix-collect-garbage -d";

      lg = "lazygit";
      gs = "git status";
      ga = "git add";
      gaa = "git add --all";
      gc = "git commit -m";
      gp = "git push";
      gpl = "git pull";
      gf = "git fetch --all";
      gb = "git branch";
      gco = "git checkout";
      gcb = "git checkout -b";
      gd = "git diff";
      gl = "git log --oneline";
      grs = "git restore --staged";
      grh = "git reset --hard";
      gst = "git stash";
      gstp = "git stash pop";
      gm = "git merge";
      grb = "git rebase";
      gcp = "git cherry-pick";
      gtl = "git tag -l";
      gta = "git tag -a";
    };

    enableCompletion = true;

    antidote = {
      enable = true;
      plugins = [
        "zdharma-continuum/fast-syntax-highlighting kind:defer"
        "zsh-users/zsh-history-substring-search"
      ];
    };

    initContent = ''
      rm_confirm() {
          local confirm

              # If not, proceed with the standard confirmation
              echo -n "Are you sure you wanna run 'rm -rf'? (yes/no): "
              read confirm
              if [ "$confirm" = "yes" ]; then
                  command rm  "$@"
              else
                  echo "Operation canceled."
              fi

      }
    '';
  };
}
