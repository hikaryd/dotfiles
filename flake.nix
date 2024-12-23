{
  description = "Hikary's system configuration";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    
    home-manager = {
      url = "github:nix-community/home-manager";
      inputs.nixpkgs.follows = "nixpkgs";
    };

    hyprland.url = "github:hyprwm/Hyprland";

    catppuccin.url = "github:catppuccin/nix";

    stylix = {
      url = "github:danth/stylix";
      inputs.nixpkgs.follows = "nixpkgs";
    };

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
  };

  outputs = { nixpkgs, home-manager, catppuccin, stylix, wut, nixgl, hyprpanel, hyprland, ... }: 
    let
      system = "x86_64-linux";
      pkgs = import nixpkgs {
        inherit system;
        config.allowUnfree = true;
        overlays = [
          nixgl.overlay
          (self: super: {
            hyprpanel = hyprpanel.packages.${system}.default.overrideAttrs (old: {
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
          ./home-manager/home.nix
          {
            home = {
              username = "hikary";
              homeDirectory = "/home/hikary";
              stateVersion = "24.11";
            };
            # Pass wut source to home-manager configuration
            _module.args = {
              wutSrc = wut;
            };
          }
          catppuccin.homeManagerModules.catppuccin
          stylix.homeManagerModules.stylix
        ];
      };
    };
}
