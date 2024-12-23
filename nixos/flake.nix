{
  description = "Basic NixOS configuration";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    nixos-generators = {
      url = "github:nix-community/nixos-generators";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { nixpkgs, nixos-generators, ... }:
    let
      system = "x86_64-linux";
    in {
      packages.${system} = {
        vm = nixos-generators.nixosGenerate {
          inherit system;
          modules = [
            ./default.nix
            ./hardware-configuration.nix
          ];
          format = "vm";
        };
      };
    };
}
