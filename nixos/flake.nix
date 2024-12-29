{
  description = "Basic NixOS configuration";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    nixos-generators = {
      url = "github:nix-community/nixos-generators";
      inputs.nixpkgs.follows = "nixpkgs";
    };
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
    hyprland = {
      url = "github:hyprwm/Hyprland";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    ghostty.url = "github:mitchellh/ghostty";
  };

  outputs = inputs@{ self, nixpkgs, nixos-generators, home-manager, catppuccin
    , stylix, nixgl, hyprpanel, hyprland, ghostty, ... }:
    let
      system = "x86_64-linux";
      baseModules = [
        { _module.args = { inherit inputs; }; }
        ./default.nix
        {
          nixpkgs = {
            config.allowUnfree = true;
            overlays = [ nixgl.overlay inputs.hyprpanel.overlay ];
          };
        }
        home-manager.nixosModules.home-manager
        {
          home-manager = {
            useGlobalPkgs = true;
            useUserPackages = true;
            extraSpecialArgs = { inherit inputs; };
            users.hikary = {
              imports = [
                ../home-manager/home.nix
                {
                  home = {
                    username = "hikary";
                    homeDirectory = "/home/hikary";
                    stateVersion = "24.11";
                  };
                }
              ];
            };
            sharedModules = [
              stylix.homeManagerModules.stylix
              catppuccin.homeManagerModules.catppuccin
            ];
          };
        }
      ];
    in {
      packages.${system} = {
        default = nixos-generators.nixosGenerate {
          inherit system;
          modules = baseModules;
          format = "vm";
        };

        iso = nixos-generators.nixosGenerate {
          inherit system;
          modules = baseModules;
          format = "iso";
        };
      };
    };
}
