{ system, ... }: {
  imports = let
    base-modules = [
      ./git.nix
      ./lazygit.nix
      ./nvim.nix
      ./yazi.nix
      ./starship.nix
      ./nushell.nix
      ./ssh.nix
      ./ghostty.nix
      ./fzf.nix
      ./bat.nix
      ./kanata.nix
    ];

    darwin-modules = [ ./yabai.nix ];

    linux-modules = [
      ./tmux.nix
      ./xdg-mimes.nix
      ./wayland
      ./onepassword.nix
      ./easyeffects.nix
      ./otd.nix
      ./fastfetch.nix
    ];

    platform-modules =
      if (system == "aarch64-darwin") then darwin-modules else linux-modules;
  in base-modules ++ platform-modules;
}
