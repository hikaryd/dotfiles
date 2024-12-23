{ pkgs, wutSrc, ... }: {
  home.packages = let
    wut = pkgs.python3Packages.buildPythonPackage {
      pname = "wut";
      version = "0.1.0";
      src = wutSrc;

      propagatedBuildInputs = with pkgs.python3Packages; [
        requests
        beautifulsoup4
        lxml
      ];

      doCheck = false;
    };
  in [ wut ];
}
