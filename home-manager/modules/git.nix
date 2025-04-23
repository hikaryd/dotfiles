{ config, pkgs, ... }: {
  programs.git = {
    enable = true;
    userName = "hikaryd";
    userEmail = "tronin371@gmail.com";
    delta = {
      enable = true;
      options = {
        features = "decorations";
        navigate = true;
        light = false;
        side-by-side = true;
        line-numbr = true;
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
    "git/hooks/prepare-commit-msg" = {
      executable = true;
      text = ''
        #!/usr/bin/env bash
        COMMIT_MSG_FILE=$1
        export OPENROUTER_API_KEY=$(cat ~/creds/open_router)
        export GOOGLE_API_KEY=$(cat ~/creds/gemini)

        COMMIT_MSG=$(${pkgs.python312}/bin/python ${config.xdg.configHome}/git/scripts/generate_commit_message.py)
        if [ -n "$COMMIT_MSG" ]; then
          echo "$COMMIT_MSG" > "$COMMIT_MSG_FILE"
        fi
      '';
    };
  };
}

