{
  description = "Hikary's system configuration";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    stylix.url = "github:danth/stylix";
    zen-browser.url = "github:0xc000022070/zen-browser-flake";
    ghostty.url = "github:ghostty-org/ghostty";
    anyrun.url = "github:fufexan/anyrun/launch-prefix";
    hyprland.url = "github:hyprwm/Hyprland";
    master.url = "github:nixos/nixpkgs/master";
    poetry2nix.url = "github:nix-community/poetry2nix";

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
    zjstatus = { url = "github:dj95/zjstatus"; };

    zj-quit = {
      url = "github:dj95/zj-quit";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    zj-smart-sessions = {
      url = "github:dj95/zj-smart-sessions";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };
  outputs = { nixpkgs, home-manager, stylix, poetry2nix, ... }@inputs:
    let
      pkgs = import nixpkgs {
        system = "x86_64-linux";
        config = {
          allowUnfree = true;
          allowUnfreePredicate = _: true;
        };
        overlays = [
          (final: prev: {
            jamesdsp = prev.jamesdsp.overrideAttrs (oldAttrs: {
              cmakeFlags = (oldAttrs.cmakeFlags or [ ])
                ++ [ "-DCMAKE_CXX_FLAGS=-Wno-error=maybe-uninitialized" ];
            });
          })
        ];
      };
      p2nix = poetry2nix.lib.mkPoetry2Nix { inherit pkgs; };
    in {
      homeConfigurations."hikary" = home-manager.lib.homeManagerConfiguration {
        inherit pkgs;
        extraSpecialArgs = { inherit inputs p2nix; };
        modules = [ ./home-manager/home.nix stylix.homeManagerModules.stylix ];
      };
    };
}
