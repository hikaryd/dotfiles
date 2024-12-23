{
  description = "NixOS configuration";

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
      pkgs = import nixpkgs {
        inherit system;
        config.allowUnfree = true;
      };
    in {
      packages.${system} = {
        iso = nixos-generators.nixosGenerate {
          inherit system;
          modules = [
            ./default.nix
            ./hardware-configuration.nix
            {
              # ISO-specific settings
              isoImage.squashfsCompression = "zstd -Xcompression-level 6";
              # Включаем поддержку UEFI
              isoImage.makeEfiBootable = true;
              # И legacy BIOS
              isoImage.makeBiosBootable = true;
            }
          ];
          format = "iso";
        };

        vm = nixos-generators.nixosGenerate {
          inherit system;
          modules = [
            ./default.nix
            ./hardware-configuration.nix
            ./vm.nix
          ];
          format = "vm";
        };
      };
    };
}
