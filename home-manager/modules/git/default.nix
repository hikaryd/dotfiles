{ inputs, pkgs, ... }: {
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
    ];
  };
  programs.git.pre-commit = {
    enable = true;
    package = inputs.pre-commit-hooks.packages.${pkgs.system}.pre-commit;
    hooks = {
      nixpkgs-fmt.enable = true;
      statix.enable = true;
      ruff.enable = true;
      shellcheck.enable = true;
      shfmt.enable = true;
      generate-commit-message = {
        enable = true;
        name = "Generate commit message";
        entry = "${pkgs.python3}/bin/python ${
            ./scripts/generate_commit_message.py
          } --operouter";
        types = [ "text" ];
        language = "system";
        pass_filenames = true;
        stages = [ "prepare-commit-msg" ];
      };
    };
  };

  xdg.configFile = {
    "git/gitk" = { source = ./gitk; };
    "git/scripts/generate_commit_message.py" = {
      source = ./scripts/generate_commit_message.py;
    };
  };
}
