{
  description = "Hikary's system configuration";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    stylix.url = "github:danth/stylix";
    home-manager = {
      url = "github:nix-community/home-manager";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    yazi-plugins = {
      url = "github:yazi-rs/plugins";
      flake = false;
    };
    nix-darwin = {
      url = "github:LnL7/nix-darwin";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    # zen-browser.url = "github:0xc000022070/zen-browser-flake";
    # zen-nebula.url =
    #   "github:JustAdumbPrsn/Nebula-A-Minimal-Theme-for-Zen-Browser";
  };
  outputs = inputs@{ self, home-manager, nix-darwin, ... }:
    let
      system = "aarch64-darwin";
      user = "tronin.egor";
    in {
      darwinConfigurations."hikary" = nix-darwin.lib.darwinSystem {
        inherit inputs;
        system = system;
        specialArgs = {
          user = user;
          inherit inputs;
        };
        modules = [
          ./darwin/configuration.nix
          home-manager.darwinModules.home-manager
          {
            home-manager = {
              useGlobalPkgs = false;
              useUserPackages = true;
              extraSpecialArgs = { inherit inputs system user; };
              users."${user}" = import ./home-manager/home.nix;
              sharedModules = [{
                nixpkgs.overlays = [
                  (final: prev: {
                    nushell = prev.nushell.overrideAttrs
                      (oldAttrs: { doCheck = false; });
                  })
                ];
              }];
            };
          }
        ];
      };
    };
}

