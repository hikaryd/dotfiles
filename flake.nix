{
  description = "Hikary's system configuration";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";

    home-manager = {
      url = "github:nix-community/home-manager";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    catppuccin.url = "github:catppuccin/nix";

    stylix.url = "github:danth/stylix";


    nixgl = {
      url = "github:nix-community/nixGL";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    hyprpanel = {
      url = "github:Jas-SinghFSU/HyprPanel";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    { nixpkgs, home-manager, catppuccin, stylix, nixgl, hyprpanel, ... }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs {
        inherit system;
        config.allowUnfree = true;
        overlays = [
          nixgl.overlay
          (self: super: {
            hyprpanel = hyprpanel.packages.${system}.default.overrideAttrs
              (old: {
                postInstall = ''
                  mv $out/share/README.md $out/share/hyprpanel-README.md
                '';
              });
          })
        ];
      };
    in {
      homeConfigurations."hikary" = home-manager.lib.homeManagerConfiguration {
        inherit pkgs;
        modules = [
          stylix.homeManagerModules.stylix
          ./home-manager/home.nix
          {
            home = {
              username = "hikary";
              homeDirectory = "/home/hikary";
              stateVersion = "24.11";
            };
          }
          catppuccin.homeManagerModules.catppuccin
        ];
      };
    };
}
