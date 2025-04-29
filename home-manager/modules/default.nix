{ system, ... }: {
  imports = let
    base-modules = [
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
    ];

    darwin-modules = [ ./yabai.nix ./onepassword-darwin.nix ];

    linux-modules =
      [ ./xdg-mimes.nix ./wayland ./onepassword.nix ./easyeffects.nix ];

    platform-modules =
      if (system == "aarch64-darwin" || system == "x86_64-darwin") then
        darwin-modules
      else
        linux-modules;
  in base-modules ++ platform-modules;
}
