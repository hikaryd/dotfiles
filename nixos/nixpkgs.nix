# Общие настройки nixpkgs для всех модулей
{ lib, ... }:

{
  nixpkgs.config = lib.mkForce {
    allowUnfree = true;
  };
}
