{ pkgs, ... }: {
  imports = let
    base-modules = [
      ./easyeffects.nix
      ./git.nix
      ./lazygit.nix
      ./nvim.nix
      ./otd.nix
      ./yazi.nix
      ./tmux.nix
      ./starship.nix
      ./nushell.nix
      ./ssh.nix
      ./ghostty.nix
      ./fzf.nix
      ./fastfetch.nix
      ./bat.nix
      ./kanata.nix
      ./xdg-mimes.nix
    ];

    darwin-modules = [ ./yabai.nix ./onepassword-darwin.nix ];

    linux-modules = [ ./wayland ./onepassword.nix ];

    platform-modules =
      if pkgs.stdenv.isDarwin then darwin-modules else linux-modules;
  in base-modules ++ platform-modules;
}
