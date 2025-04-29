{ lib, ... }: {
  programs.ghostty = {
    enable = false;

    settings = {
      background-opacity = lib.mkForce 0.8;
      minimum-contrast = 1.1;
      font-size = 10;
      # font = "Fira Code";
      term = "xterm-256color";
      bold-is-bright = true;

      command = "${../../scripts/tmux-session.sh}";
      linux-cgroup = "single-instance";

      theme = "stylix";
      background-blur-radius = 20;

      confirm-close-surface = false;
      mouse-hide-while-typing = true;

      window-padding-x = 10;
      window-padding-y = 10;
      window-padding-balance = true;
      window-inherit-working-directory = true;
      window-inherit-font-size = true;

      scrollback-limit = 1000000;
      custom-shader-animation = true;
    };
  };
}
