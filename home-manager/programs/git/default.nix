{pkgs, ...}: {
  programs.git = {
    enable = true;
    userName = "Hikary";
    userEmail = "hikary@hikary.me";
    
    delta = {
      enable = true;
      options = {
        features = "decorations";
        navigate = true;
        light = false;
        side-by-side = true;
        line-numbers = true;
        syntax-theme = "Catppuccin-mocha";
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
      lg = "log --color --graph --pretty=format:'%Cred%h%Creset -%C(yellow)%d%Creset %s %Cgreen(%cr) %C(bold blue)<%an>%Creset' --abbrev-commit";
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

  xdg.configFile = {
    "git/gitk" = {
      source = ./gitk;
    };
    "git/hooks" = {
      source = ./hooks;
      recursive = true;
    };
  };
}
