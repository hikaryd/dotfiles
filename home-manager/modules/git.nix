{ config, pkgs, lib, ... }: {
  programs.git = {
    enable = true;
    userName = "Hikary";
    userEmail = "hikary@proton.me";
    delta = {
      enable = true;
      options = {
        features = "decorations";
        navigate = true;
        light = false;
        side-by-side = true;
        line-numbers = true;
      };
    };
    aliases = {
      st = "status";
      ci = "commit";
      co = "checkout";
      br = "branch";
      unstage = "reset HEAD --";
      last = "log -1 HEAD";
      visual = "!gitk";
      lg =
        "log --color --graph --pretty=format:'%Cred%h%Creset -%C(yellow)%d%Creset %s %Cgreen(%cr) %C(bold blue)<%an>%Creset' --abbrev-commit";
    };
    extraConfig = {
      init.defaultBranch = "main";
      core = {
        editor = "nvim";
        whitespace = "trailing-space,space-before-tab";
        hooksPath = "${config.xdg.configHome}/git/hooks";
      };
      color = {
        ui = true;
        diff = "auto";
        status = "auto";
        branch = "auto";
      };
      pull.rebase = true;
      push.default = "simple";
      merge.conflictstyle = "diff3";
      diff.colorMoved = "default";
    };
    ignores = [
      ".DS_Store"
      "*.swp"
      "*.swo"
      "*~"
      "*.log"
      ".env"
      ".direnv/"
      "result"
      "result-*"
      ".pre-commit-config.yaml"
    ];
  };

  xdg.configFile = {
    "git/gitk" = { source = ../configs/git/gitk; };
    "git/scripts/generate_commit_message.py" = {
      executable = true;
      source = ../configs/git/scripts/generate_commit_message.py;
    };
    "git/hooks/pre-commit" = {
      executable = true;
      text = ''
        #!/usr/bin/env bash
        python_files=$(git diff --cached --name-only --diff-filter=ACM | grep '\.py$' || true)
        if [ -n "$python_files" ]; then
          ${pkgs.ruff}/bin/ruff check $python_files
        fi
        exit 0
      '';
    };
    "git/hooks/prepare-commit-msg" = {
      executable = true;
      text = ''
        #!/usr/bin/env zsh
        COMMIT_MSG_FILE=$1
        export OPENROUTER_API_KEY=$(cat ~/creds/open_router)

        COMMIT_MSG=$(${pkgs.python312}/bin/python ${config.xdg.configHome}/git/scripts/generate_commit_message.py --operouter)
        if [ -n "$COMMIT_MSG" ]; then
          echo "$COMMIT_MSG" > "$COMMIT_MSG_FILE"
        fi
      '';
    };
  };
}

