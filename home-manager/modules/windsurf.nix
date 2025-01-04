{ pkgs, ... }:

let
  version = "1.1.0";
  hash = "c418a14b63f051e96dafb37fe06f1fe0b10ba3c8";
in {
  home.packages = [
    (pkgs.callPackage
      "${pkgs.path}/pkgs/applications/editors/vscode/generic.nix" rec {
        commandLineArgs = "";
        useVSCodeRipgrep = pkgs.stdenv.hostPlatform.isDarwin;
        inherit version;
        pname = "windsurf";
        executableName = "windsurf";
        longName = "Windsurf";
        shortName = "windsurf";
        src = pkgs.fetchurl {
          url =
            "https://windsurf-stable.codeiumdata.com/linux-x64/stable/${hash}/Windsurf-linux-x64-${version}.tar.gz";
          hash = "sha256-fsDPzHtAmQIfFX7dji598Q+KXO6A5F9IFEC+bnmQzVU=";
        };
        sourceRoot = "Windsurf";
        tests = pkgs.nixosTests.vscodium;
        updateScript = "nil";
        meta = { description = "The first agentic IDE, and then some"; };
      })
  ];
}
