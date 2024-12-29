{ inputs, pkgs, ... }:

{
  imports = [ ./hardware-configuration.nix ];

  boot = {
    loader = {
      systemd-boot = {
        enable = true;
        configurationLimit = 5;
      };
      efi.canTouchEfiVariables = true;
    };
    kernelParams = [
      "quiet"
      "splash"
      "mitigations=off"
      "amd_pstate=active"
      "amdgpu.ppfeaturemask=0xffffffff"
      "amdgpu.dcfeaturemask=0xffffffff"
      "transparent_hugepage=always"
    ];
    kernelModules = [ "kvm-amd" ];
    kernel.sysctl = {
      "vm.swappiness" = 10;
      "vm.vfs_cache_pressure" = 50;
      "net.core.rmem_max" = 2500000;
      "net.core.wmem_max" = 2500000;
      "kernel.nmi_watchdog" = 0;
    };
    tmp = {
      cleanOnBoot = true;
      useTmpfs = true;
      tmpfsSize = "50%";
    };
    initrd.kernelModules = [ "amdgpu" ];
  };

  networking = {
    networkmanager.enable = true;
    hostName = "hikary";
  };

  time.timeZone = "Europe/Moscow";
  i18n = {
    defaultLocale = "en_US.UTF-8";
    extraLocaleSettings = {
      LC_ADDRESS = "ru_RU.UTF-8";
      LC_IDENTIFICATION = "ru_RU.UTF-8";
      LC_MEASUREMENT = "ru_RU.UTF-8";
      LC_MONETARY = "ru_RU.UTF-8";
      LC_NAME = "ru_RU.UTF-8";
      LC_NUMERIC = "ru_RU.UTF-8";
      LC_PAPER = "ru_RU.UTF-8";
      LC_TELEPHONE = "ru_RU.UTF-8";
      LC_TIME = "ru_RU.UTF-8";
    };
  };

  console = {
    font = "ter-v32n";
    keyMap = "us";
    packages = [ pkgs.terminus_font ];
  };

  services.xserver.enable = false;

  programs.hyprland = {
    enable = true;
    xwayland.enable = true;
    package =
      inputs.hyprland.packages.${pkgs.stdenv.hostPlatform.system}.hyprland;
    portalPackage =
      inputs.hyprland.packages.${pkgs.stdenv.hostPlatform.system}.xdg-desktop-portal-hyprland;
  };

  systemd.services.emptty = {
    enable = true;
    description = "emptty display manager";
    after = [
      "systemd-user-sessions.service"
      "getty@tty7.service"
      "plymouth-quit.service"
    ];
    conflicts = [ "getty@tty7.service" ];
    serviceConfig = {
      Type = "simple";
      Environment = "XDG_SESSION_TYPE=wayland";
      ExecStart = "${pkgs.emptty}/bin/emptty";
      TTYPath = "/dev/tty7";
      TTYReset = "yes";
      TTYVHangup = "yes";
      TTYVTDisallocate = "yes";
      Restart = "always";
    };
    wantedBy = [ "multi-user.target" ];
  };

  environment.systemPackages = with pkgs; [
    emptty
    wayland
    xdg-utils
    grim
    slurp
    wl-clipboard
    vim
    wget
    git
    pciutils
    usbutils
    docker-compose
    blueman
    networkmanagerapplet
    radeontop
    corectrl
    zenmonitor
    rocmPackages.rocm-smi

    python312
    python312Packages.uv
    python312Packages.greenlet
    nodejs_20
    gcc
    stdenv.cc.cc.lib
  ];

  environment.sessionVariables = {
    NIXOS_OZONE_WL = "1";
    XDG_SESSION_TYPE = "wayland";
    WLR_NO_HARDWARE_CURSORS = "1";
  };

  hardware = {
    graphics = {
      enable = true;
      enable32Bit = true;
      extraPackages = with pkgs; [
        rocmPackages.clr
        amdvlk
        vaapiVdpau
        libvdpau-va-gl
        mesa.drivers
        vulkan-loader
        vulkan-validation-layers
        vulkan-tools
        vulkan-headers
        mesa-demos
      ];
      extraPackages32 = with pkgs.pkgsi686Linux; [
        libva
        vaapiVdpau
        vulkan-loader
      ];
    };
    enableRedistributableFirmware = true;
  };

  security.rtkit.enable = true;
  services.pipewire = {
    enable = true;
    alsa.enable = true;
    alsa.support32Bit = true;
    pulse.enable = true;
    jack.enable = true;
  };

  virtualisation = {
    docker = {
      enable = true;
      daemon.settings = { features = { buildkit = true; }; };
      autoPrune = {
        enable = true;
        dates = "weekly";
      };
    };
    oci-containers.backend = "docker";
  };

  hardware.cpu.amd.updateMicrocode = true;

  services = {
    thermald.enable = true;
    tlp = {
      enable = true;
      settings = {
        CPU_SCALING_GOVERNOR_ON_AC = "performance";
        CPU_SCALING_GOVERNOR_ON_BAT = "powersave";
      };
    };
    power-profiles-daemon.enable = false;
    udisks2.enable = true;
    dbus.enable = true;
    upower.enable = true;
    acpid.enable = true;
    fstrim.enable = true;
    smartd.enable = false;
    fwupd.enable = true;
  };

  hardware.bluetooth = {
    enable = true;
    powerOnBoot = true;
    settings = { General = { Enable = "Source,Sink,Media,Socket"; }; };
  };
  services.blueman.enable = true;
  hardware.pulseaudio.enable = false;

  programs.zsh.enable = true;

  users.users.hikary = {
    isNormalUser = true;
    description = "hikary";
    extraGroups = [
      "networkmanager"
      "wheel"
      "video"
      "audio"
      "input"
      "docker"
      "scanner"
      "lp"
    ];
    initialPassword = "password123";
    shell = pkgs.zsh;
  };

  nixpkgs.config.allowUnfree = true;

  nix = {
    settings = {
      auto-optimise-store = true;
      experimental-features = [ "nix-command" "flakes" ];
    };
    gc = {
      automatic = true;
      dates = "weekly";
      options = "--delete-older-than 7d";
    };
  };

  zramSwap = {
    enable = true;
    algorithm = "zstd";
    memoryPercent = 50;
  };

  services.udev.extraRules = ''
    ACTION=="add|change", KERNEL=="[sv]d[a-z]", ATTR{queue/scheduler}="bfq"
    ACTION=="add|change", KERNEL=="nvme[0-9]n[0-9]", ATTR{queue/scheduler}="none"
  '';

  security = {
    polkit.enable = true;
    sudo.wheelNeedsPassword = false;
    pam = {
      services = { login = { enableGnomeKeyring = true; }; };
      loginLimits = [
        {
          domain = "@wheel";
          type = "soft";
          item = "nofile";
          value = "524288";
        }
        {
          domain = "@wheel";
          type = "hard";
          item = "nofile";
          value = "1048576";
        }
      ];
    };
  };

  system.stateVersion = "24.05";
}
