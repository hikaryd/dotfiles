{ pkgs, lib, ... }: {
  services.jankyborders = {
    enable = lib.mkForce false;
    package = pkgs.jankyborders;
    width = 5.0;
    active_color = "#cdd6f4";
    inactive_color = "#1e1e2e";
    hidpi = true;
    ax_focus = true;
  };
}
