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

    wut = {
      url = "github:shobrook/wut";
      flake = false;
    };

    nixgl = {
      url = "github:nix-community/nixGL";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    hyprpanel = {
      url = "github:Jas-SinghFSU/HyprPanel";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    emptty = {
      url = "github:tvrzna/emptty";
      flake = false;
    };

    nixos-generators = {
      url = "github:nix-community/nixos-generators";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { nixpkgs, home-manager, catppuccin, stylix, wut, nixgl, hyprpanel
    , emptty, nixos-generators, ... }:
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
          (final: prev: {
            emptty = prev.callPackage ./packages/emptty.nix { src = emptty; };
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
            _module.args = { wutSrc = wut; };
          }
          catppuccin.homeManagerModules.catppuccin
        ];
      };

      packages.${system} = {
        iso = nixos-generators.nixosGenerate {
          inherit system pkgs;
          modules = [
            ./nixos/default.nix
            ./nixos/hardware-configuration.nix
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
          inherit system pkgs;
          modules = [
            ./nixos/default.nix
            ./nixos/hardware-configuration.nix
            ./nixos/vm.nix
          ];
          format = "vm";
        };
      };
    };
}
