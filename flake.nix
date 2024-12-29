{
  description = "Hikary's system configuration";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    catppuccin.url = "github:catppuccin/nix";
    stylix.url = "github:danth/stylix";
    ghostty = { url = "github:ghostty-org/ghostty"; };

    home-manager = {
      url = "github:nix-community/home-manager";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    nixgl = {
      url = "github:nix-community/nixGL";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    hyprpanel = {
      url = "github:Jas-SinghFSU/HyprPanel";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = inputs@{ nixpkgs, home-manager, catppuccin, stylix, nixgl, hyprpanel, ghostty, ... }:
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs {
        inherit system;
        config.allowUnfree = true;
        overlays = [ nixgl.overlay inputs.hyprpanel.overlay ];
      };
    in {
      homeConfigurations."hikary" = home-manager.lib.homeManagerConfiguration {
        inherit pkgs;
        extraSpecialArgs = {
          inherit inputs;
          inherit stylix nixgl ghostty;
        };
        modules = [
          stylix.homeManagerModules.stylix
          ./home-manager/home.nix
          catppuccin.homeManagerModules.catppuccin
        ];
      };
    };
}
