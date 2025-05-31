{ ... }: {
  programs.lazygit.enable = true;
  programs.lazygit.settings = {
    customCommands = [{
      "key" = "C";
      "command" = "git commit";
      "description" = "Commit (open in nvim)";
      "output" = "terminal";
      "loadingText" = "Opening nvim...";
      "context" = "files";
    }];
  };
}
