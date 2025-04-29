{
  description = "Hikary's system configuration";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    stylix.url = "github:danth/stylix";
    anyrun.url = "github:fufexan/anyrun/launch-prefix";
    hyprpanel.url = "github:Jas-SinghFSU/HyprPanel";
    home-manager = {
      url = "github:nix-community/home-manager";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    yazi-plugins = {
      url = "github:yazi-rs/plugins";
      flake = false;
    };
    nixgl = {
      url = "github:nix-community/nixGL";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    niri = {
      url = "github:sodiboo/niri-flake";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    zen-browser.url = "github:MarceColl/zen-browser-flake";
    # https://github.com/JustAdumbPrsn/Nebula-A-Minimal-Theme-for-Zen-Browser
    nix-darwin = {
      url = "github:LnL7/nix-darwin";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };
  outputs = inputs@{ self, nixpkgs, home-manager, nix-darwin, ... }:
    let
      system = builtins.currentSystem;
      pkgs = import nixpkgs {
        inherit system;
        config = { allowUnfree = true; };
        overlays = [
          inputs.hyprpanel.overlay
          inputs.nixgl.overlay
          inputs.niri.overlays.niri
        ];
      };
    in {
      homeConfigurations."hikary" = home-manager.lib.homeManagerConfiguration {
        inherit pkgs;
        extraSpecialArgs = { inherit inputs system; };
        modules = [ ./home-manager/home.nix ];
      };

      darwinConfigurations."hikary-mac" = nix-darwin.lib.darwinSystem {
        inherit inputs;
        system = "aarch64-darwin";
        modules = [
          ./darwin/configuration.nix
          home-manager.darwinModules.home-manager
          {
            home-manager = {
              useGlobalPkgs = false;
              useUserPackages = true;
              extraSpecialArgs = { inherit inputs system; };
              users."tronin.egor" = import ./home-manager/home.nix;
            };
          }
        ];
      };
    };
}
