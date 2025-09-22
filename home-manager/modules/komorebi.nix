{ pkgs, config, ... }: {
  home.packages = [ pkgs.komorebi ];

  launchd.agents.komorebi = {
    enable = true;
    config = {
      Label = "dev.komorebi";
      ProgramArguments = [ "komorebi" ];
      #/Users/tronin.egor/.cargo/bin/komorebi
      RunAtLoad = true;
      KeepAlive = { SuccessfulExit = false; };
      EnvironmentVariables = { LANG = "en_US.UTF-8"; };
      StandardOutPath =
        "${config.home.homeDirectory}/Library/Logs/komorebi.log";
      StandardErrorPath =
        "${config.home.homeDirectory}/Library/Logs/komorebi.err";
      ProcessType = "Interactive";
    };
    path = [ pkgs.coreutils pkgs.bash ];
  };
}
