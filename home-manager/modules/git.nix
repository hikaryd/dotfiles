{ config, ... }: {
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
      init.defaultBranch = "dev";
      core = {
        editor = "nvim";
        compression = 9;
        whitespace = "error";
        preloadindex = true;
        hooksPath = "${config.xdg.configHome}/git/hooks";
      };
      push = {
        default = "current";
        autoSetupRemote = true;
        followTags = true;
      };
      status = {
        showStash = true;
        branch = true;
        showUntrackedFiles = "all";
      };
      advice = {
        addEmptyPathspec = false;
        pushNonFastForward = false;
        statushHints = false;
      };
      color = {
        ui = true;
        diff = "auto";
        status = "auto";
        branch = "auto";
      };
      pull = {
        default = "current";
        rebase = true;
      };
      rebase = {
        autoStash = true;
        missingCommitsCheck = true;
      };
      merge.conflictstyle = "diff3";
      diff.colorMoved = "default";
    };
    ignores = [
      ".DS_Store"
      "*.swp"
      "*.swo"
      ".claude"
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
    "git/hooks/prepare-commit-msg" = {
      executable = true;
      text = ''
        #!/usr/bin/env bash
        COMMIT_MSG_FILE=$1

        COMMIT_MSG=$(${../../scripts/ai_helper} --mode commit)
        if [ -n "$COMMIT_MSG" ]; then
          echo "$COMMIT_MSG" > "$COMMIT_MSG_FILE"
        fi
      '';
    };
  };
}

