{ system, ... }: {
  imports = (if system == "x86_64-linux" then [ ./wayland ] else [ ]) ++ [
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
    ./onepassword.nix
  ];
}
