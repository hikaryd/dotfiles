{ config, ... }: {
  programs.git = {
    enable = true;
    settings = {
      user = {
        name = "Egor Tronin";
        email = "egor.tronin@x5.ru";
      };
      alias = {
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
      "requirements"
    ];
  };

  programs.delta = {
    enable = true;
    enableGitIntegration = true;
    options = {
      features = "decorations";
      navigate = true;
      light = false;
      side-by-side = true;
      line-numbr = true;
    };
  };

  xdg.configFile = {
    "git/gitk" = { source = ../configs/git/gitk; };
    "git/hooks/prepare-commit-msg" = {
      executable = true;
      text = ''
        #!/bin/sh
        COMMIT_MSG_FILE="$1"

        AI_COMMIT=$(${
          ../../scripts/ai_helper
        } --mode commit --provider x5 2>/dev/null)

        BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null)

        TICKET=$(printf '%s' "$BRANCH" | grep -Eoi 'ATS-[0-9]+' | head -n 1)

        if [ -z "$TICKET" ]; then
          NUM=$(printf '%s' "$BRANCH" | grep -Eo '[0-9]+' | head -n 1)
          if [ -n "$NUM" ]; then
            TICKET="ATS-$NUM"
          fi
        fi

        if [ -n "$TICKET" ]; then
          TICKET=$(printf '%s' "$TICKET" | tr '[:lower:]' '[:upper:]')
        fi

        prepend_ticket_to_file() {
          if [ -n "$TICKET" ]; then
            if ! head -n 1 "$COMMIT_MSG_FILE" | grep -Eq "^$TICKET([[:space:]]|$)"; then
              awk -v prefix="$TICKET " 'NR==1{print prefix $0; next} {print}' \
                "$COMMIT_MSG_FILE" > "$COMMIT_MSG_FILE.tmp" \
                && mv "$COMMIT_MSG_FILE.tmp" "$COMMIT_MSG_FILE"
            fi
          fi
        }

        if [ -n "$AI_COMMIT" ]; then
          TITLE=$(printf '%s' "$AI_COMMIT" | head -n 1)
          DESCRIPTION=$(printf '%s' "$AI_COMMIT" | tail -n +3)

          if [ -n "$TICKET" ] && ! printf '%s' "$TITLE" | grep -Eq "^$TICKET([[:space:]]|$)"; then
            TITLE="$TICKET $TITLE"
          fi

          if [ -n "$DESCRIPTION" ]; then
            printf "%s\n\n%s\n" "$TITLE" "$DESCRIPTION" > "$COMMIT_MSG_FILE"
          else
            printf "%s\n" "$TITLE" > "$COMMIT_MSG_FILE"
          fi
        else
          prepend_ticket_to_file
        fi
      '';
    };
  };
}

