{ ... }: {
  programs.lazygit.enable = true;
  programs.lazygit.settings = {
    customCommands = [{
      "key" = "C";
      "command" = "git commit";
      "description" = "Commit (open in nvim)";
      "subprocess" = true;
      "loadingText" = "Opening nvim...";
      "context" = "files";
    }];
  };
}
