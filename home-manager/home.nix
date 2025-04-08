{ pkgs, inputs, lib, ... }: {
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

  programs.bash = {
    enable = true;
    bashrcExtra = ''
      export LANG="en_US.UTF-8"
      export LC_ALL="en_US.UTF-8"
    '';
  };

  home.file.".xprofile".text = ''
    if [ -f "$HOME/.nix-profile/etc/profile.d/hm-session-vars.sh" ]; then
      . "$HOME/.nix-profile/etc/profile.d/hm-session-vars.sh"
    fi
  '';

  home.sessionVariables = {
    _JAVA_AWT_WM_NONEREPARENTING = "1";
    DISABLE_QT5_COMPAT = "0";
    ANKI_WAYLAND = "1";
    QT_QPA_PLATFORMTHEME = "qt5ct";
    QT_STYLE_OVERRIDE = "kvantum";
    AMD_VULKAN_ICD = "RADV";
    LIBVA_DRIVER_NAME = "radeonsi";
    MESA_LOADER_DRIVER_OVERRIDE = "amdgpu";
    __GLX_VENDOR_LIBRARY_NAME = "amdgpu";
    GBM_BACKEND = "amdgpu";

    XDG_SESSION_TYPE = "wayland";

    GDK_BACKEND = "wayland";
    MOZ_ENABLE_WAYLAND = "1";
    SDL_VIDEODRIVER = "wayland";
    CLUTTER_BACKEND = "wayland";
    ELECTRON_OZONE_PLATFORM_HINT = "wayland";

    QT_QPA_PLATFORM = "wayland;xcb";
    QT_AUTO_SCREEN_SCALE_FACTOR = "1";
    QT_WAYLAND_DISABLE_WINDOWDECORATION = "1";

    XCURSOR_THEME = "Bibata-Modern-Ice";
    XCURSOR_SIZE = "20";

    TERM = "ghostty";
  };

  systemd.user.services = {
    onepassword = {
      Unit = {
        Description = "1Password GUI Helper";
        After = [ "graphical-session-pre.target" ];
        PartOf = [ "graphical-session.target" ];
      };
      Service = {
        Type = "simple";
        ExecStart = "/usr/bin/1password --silent";
        Restart = "on-failure";
        RestartSec = 5;
      };
      Install = { WantedBy = [ "graphical-session.target" ]; };
    };

    kanata = {
      Unit = {
        Description = "Kanata Key Remapper";
        After = [ "graphical-session-pre.target" ];
        PartOf = [ "graphical-session.target" ];
      };
      Service = {
        Type = "simple";
        ExecStart = "${pkgs.kanata}/bin/kanata";
        Restart = "on-failure";
        RestartSec = 5;
      };
      Install = { WantedBy = [ "graphical-session.target" ]; };
    };

    jamesdsp = {
      Unit = {
        Description = "JamesDSP Service";
        After = [ "graphical-session-pre.target" ];
        PartOf = [ "graphical-session.target" ];
      };
      Service = {
        Type = "simple";
        ExecStart = "${pkgs.jamesdsp}/bin/jamesdsp";
        Restart = "on-failure";
        RestartSec = 5;
      };
      Install = { WantedBy = [ "graphical-session.target" ]; };
    };

    easyeffects = {
      Unit = {
        Description = "EasyEffects Service";
        After = [ "graphical-session-pre.target" ];
        PartOf = [ "graphical-session.target" ];
      };
      Service = {
        Type = "dbus";
        BusName = "com.github.wwmm.easyeffects";
        ExecStart =
          "${pkgs.easyeffects}/bin/easyeffects --gapplication-service";
        Restart = "on-failure";
        RestartSec = 5;
      };
      Install = { WantedBy = [ "graphical-session.target" ]; };
    };

    polkitGnomeAgent = {
      Unit = {
        Description = "Polkit GNOME Authentication Agent";
        After = [ "graphical-session-pre.target" ];
        PartOf = [ "graphical-session.target" ];
      };
      Service = {
        Type = "simple";
        ExecStart = "/usr/lib/polkit-gnome/polkit-gnome-authentication-agent-1";
        Restart = "on-failure";
        RestartSec = 5;
      };
      Install = { WantedBy = [ "graphical-session.target" ]; };
    };
  };

}
