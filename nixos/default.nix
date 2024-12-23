{ pkgs, ... }: {
  imports =
    [ ./hardware-configuration.nix ./plymouth-theme.nix ../modules/emptty.nix ];

  # Загрузчик
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
      "mitigations=off" # Улучшение производительности (отключение патчей Spectre/Meltdown)
      "amd_pstate=active" # Активный режим управления питанием AMD
      "amdgpu.ppfeaturemask=0xffffffff" # Включаем все функции AMD GPU
      "amdgpu.dcfeaturemask=0xffffffff" # Включаем все функции дисплейного контроллера
    ];
    kernelModules = [ "kvm-amd" "amdgpu" ];
    kernel.sysctl = {
      "vm.swappiness" = 10; # Уменьшаем использование swap
      "vm.vfs_cache_pressure" = 50; # Оптимизация кэша
      "net.core.rmem_max" = 2500000; # Увеличиваем буфер приема
      "net.core.wmem_max" = 2500000; # Увеличиваем буфер отправки
      "kernel.nmi_watchdog" = 0; # Отключаем watchdog для экономии энергии
    };
  };

  networking = {
    networkmanager.enable = true;
    hostName = "hikary";
  };

  # Часовой пояс
  time.timeZone = "Europe/Moscow";

  # Локализация
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

  # Настройка консоли
  console = {
    font = "ter-v32n";
    keyMap = "ru";
    packages = [ pkgs.terminus_font ];
  };

  # Настройка X11 и Wayland
  services.xserver = {
    enable = true;
    displayManager = { emptty = { enable = true; }; };
    xkb = {
      layout = "us,ru";
      options = "grp:alt_shift_toggle";
    };
  };

  # Поддержка графики
  hardware.graphics = {
    enable = true;
    extraPackages = with pkgs; [
      amdvlk
      rocmPackages.clr
      libvdpau-va-gl
      vaapiVdpau
      rocmPackages.runtime
      clinfo
    ];
  };

  # Настройка звука через pipewire
  services.pipewire = {
    enable = true;
    alsa.enable = true;
    alsa.support32Bit = true;
    pulse.enable = true;
    jack.enable = true;
  };

  # Docker
  virtualisation = {
    docker = {
      enable = true;
      daemon.settings = { features = { buildkit = true; }; };
    };
    oci-containers.backend = "docker";
  };

  # Оптимизации для AMD
  hardware.cpu.amd.updateMicrocode = true;

  # Службы для ноутбука
  services.thermald.enable = true;
  services.tlp = {
    enable = true;
    settings = {
      CPU_SCALING_GOVERNOR_ON_AC = "performance";
      CPU_SCALING_GOVERNOR_ON_BAT = "powersave";
    };
  };

  # Bluetooth
  hardware.bluetooth = {
    enable = true;
    powerOnBoot = true;
    settings = { General = { Enable = "Source,Sink,Media,Socket"; }; };
  };
  services.blueman.enable = true;

  # Фоновые службы
  services.fstrim.enable = true;
  services.smartd.enable = true;
  services.fwupd.enable = true;

  # Включаем zsh
  programs.zsh.enable = true;

  # Пользователь
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
    shell = pkgs.zsh;
  };

  # Разрешаем несвободные пакеты
  nixpkgs.config.allowUnfree = true;

  # Системные пакеты (только те, которые нужны на системном уровне)
  environment.systemPackages = with pkgs; [
    # Системные утилиты
    vim # Базовый редактор, может понадобиться в emergency shell
    wget # Базовая утилита для скачивания
    git # Нужен для обновления системы

    # Драйверы и утилиты для железа
    pciutils
    usbutils
    lshw
    dmidecode

    # Утилиты для работы с дисками
    parted
    gparted
    smartmontools

    # Системные демоны и их утилиты
    docker-compose
    blueman
    networkmanagerapplet

    # AMD утилиты
    radeontop
    corectrl
    zenmonitor
    rocmPackages.rocm-smi
  ];

  # Настройки системы безопасности и PAM
  security = {
    rtkit.enable = true;
    polkit.enable = true;
    pam = {
      services = {
        login = {
          enableGnomeKeyring = true;
          fprintAuth = true;
        };
        sudo = {
          fprintAuth = true;
          text = ''
            auth sufficient pam_fprintd.so
            auth sufficient pam_unix.so try_first_pass nullok
            auth required pam_deny.so
          '';
        };
        lightdm.fprintAuth = true;
        xscreensaver.fprintAuth = true;
        "gdm-fingerprint" = {
          text = ''
            auth     required       pam_fprintd.so
            account  required       pam_unix.so
            password required       pam_unix.so nullok sha512
            session  required       pam_unix.so
          '';
        };
      };
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

  system.stateVersion = "24.11";
}
