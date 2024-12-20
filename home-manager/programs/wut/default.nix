{ pkgs, ... }: {
  home.packages = let
    wutSrc = builtins.fetchTarball {
      url = "https://github.com/shobrook/wut/archive/master.tar.gz";
    };

    wut = pkgs.python3.pkgs.buildPythonApplication {
      pname = "wut";
      version = "0.1.0";
      src = wutSrc;

      propagatedBuildInputs = with pkgs.python3.pkgs; [
        requests
        openai
        anthropic
        python-dotenv
        rich
        typer
        ollama
        psutil
      ];
    };
  in [ wut ];
}
