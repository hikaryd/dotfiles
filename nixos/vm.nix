# Конфигурация для виртуальной машины
{ config, lib, pkgs, modulesPath, ... }:

{
  imports = [ (modulesPath + "/virtualisation/qemu-vm.nix") ];

  # Настройки виртуализации
  virtualisation = {
    diskSize = 20 * 1024;  # 20GB
    memorySize = 4 * 1024; # 4GB
    useEFIBoot = true;
    useHostCerts = true;
  };
}
