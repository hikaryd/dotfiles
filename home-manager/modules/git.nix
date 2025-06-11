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
        #!/bin/sh
        COMMIT_MSG_FILE=$1

        AI_COMMIT=$(${../../scripts/ai_helper} --mode commit 2>/dev/null)

        if [ -n "$AI_COMMIT" ]; then
          BRANCH=$(git rev-parse --abbrev-ref HEAD)
          PREFIX=$(echo "$BRANCH" | awk -F'[/_]' '{print $2}')
          
          if [ -n "$PREFIX" ]; then
            TITLE=$(echo "$AI_COMMIT" | head -n 1)
            DESCRIPTION=$(echo "$AI_COMMIT" | tail -n +3)
            
            PREFIXED_TITLE="$PREFIX $TITLE"
            
            if [ -n "$DESCRIPTION" ]; then
              printf "%s\n\n%s\n" "$PREFIXED_TITLE" "$DESCRIPTION" > "$COMMIT_MSG_FILE"
            else
              echo "$PREFIXED_TITLE" > "$COMMIT_MSG_FILE"
            fi
          else
            echo "$AI_COMMIT" > "$COMMIT_MSG_FILE"
          fi
        else
          BRANCH=$(git rev-parse --abbrev-ref HEAD)
          PREFIX=$(echo "$BRANCH" | awk -F'[/_]' '{print $2}')
          if [ -n "$PREFIX" ]; then
            awk -v prefix="$PREFIX " 'NR==1{print prefix $0; next} {print}' "$COMMIT_MSG_FILE" > "$COMMIT_MSG_FILE.tmp" && mv "$COMMIT_MSG_FILE.tmp" "$COMMIT_MSG_FILE"
          fi
        fi
      '';
    };
  };
}

