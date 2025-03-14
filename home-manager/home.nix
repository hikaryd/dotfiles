{ pkgs, inputs, ... }: {
  home = {
    username = "hikary";
    homeDirectory = "/home/hikary";
    stateVersion = "24.11";
  };

  imports = [ ./modules ./theme.nix ./packages.nix ];
  programs.home-manager.enable = true;
  nixGL = {
    packages = inputs.nixgl.packages.${pkgs.system};
    defaultWrapper = "mesa";
  };

  xdg.configFile."environment.d/envvars.conf".text = ''
    PATH="$HOME/.nix-profile/bin:/nix/var/nix/profiles/default/bin:/home/hikary/.local/bin:$HOME/.cargo/bin:$PATH"
  '';

  programs.bash = {
    enable = true;
    bashrcExtra = ''
      export PATH="$HOME/.nix-profile/bin:/nix/var/nix/profiles/default/bin:/home/hikary/.local/bin:$HOME/.cargo/bin:$PATH"
      export LANG="en_US.UTF-8"
      export LC_ALL="en_US.UTF-8"
    '';
  };

  home.file.".xprofile".text = ''
    if [ -f "$HOME/.nix-profile/etc/profile.d/hm-session-vars.sh" ]; then
      . "$HOME/.nix-profile/etc/profile.d/hm-session-vars.sh"
    fi
  '';

  home.sessionPath = [ "$HOME/.nix-profile/bin" ];
  home.sessionVariables = {
    PATH = "$HOME/.nix-profile/bin:$PATH";

    _JAVA_AWT_WM_NONEREPARENTING = "1";
    DISABLE_QT5_COMPAT = "0";
    GDK_BACKEND = "wayland";
    ANKI_WAYLAND = "1";
    WLR_DRM_NO_ATOMIC = "1";
    QT_AUTO_SCREEN_SCALE_FACTOR = "1";
    QT_WAYLAND_DISABLE_WINDOWDECORATION = "1";
    QT_QPA_PLATFORM = "wayland;xcb";
    QT_QPA_PLATFORMTHEME = "qt5ct";
    QT_STYLE_OVERRIDE = "kvantum";
    MOZ_ENABLE_WAYLAND = "1";
    WLR_BACKEND = "drm";
    WLR_RENDERER = "vulkan";
    WLR_NO_HARDWARE_CURSORS = "1";
    XDG_SESSION_TYPE = "wayland";
    SDL_VIDEODRIVER = "wayland";
    CLUTTER_BACKEND = "wayland";
    GBM_BACKEND = "amdgpu";
    AMD_VULKAN_ICD = "RADV";
    LIBVA_DRIVER_NAME = "radeonsi";
    WLR_RENDERER_ALLOW_SOFTWARE = "1";
    MESA_LOADER_DRIVER_OVERRIDE = "amdgpu";
    __GLX_VENDOR_LIBRARY_NAME = "amdgpu";
  };
}
