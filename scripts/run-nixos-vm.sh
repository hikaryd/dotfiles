#!/usr/bin/env bash

VM_NAME="nixos-test"
MEMORY="4G"
CPUS="4"
DISK="nixos.qcow2"

# Проверяем наличие образа
if [ ! -f "$DISK" ]; then
    echo "Образ $DISK не найден!"
    echo "Хотите создать новый образ? [y/N]"
    read -r response
    if [[ "$response" =~ ^([yY][eE][sS]|[yY])+$ ]]; then
        echo "Создаем новый образ..."
        nix run nixpkgs#nixos-generators -- --format qcow -c ./nixos/default.nix
    else
        exit 1
    fi
fi

# Запускаем VM с оптимальными настройками для десктопа
qemu-system-x86_64 \
    -name "$VM_NAME" \
    -machine type=q35,accel=kvm \
    -cpu host \
    -smp "$CPUS" \
    -m "$MEMORY" \
    -drive file="$DISK",if=virtio,cache=writeback,discard=unmap,format=qcow2 \
    -display gtk,gl=on \
    -device virtio-vga-gl \
    -device virtio-tablet-pci \
    -device virtio-keyboard-pci \
    -device intel-hda \
    -device hda-duplex \
    -device virtio-net-pci,netdev=net0 \
    -netdev user,id=net0 \
    -usb \
    -device qemu-xhci \
    -object rng-random,filename=/dev/urandom,id=rng0 \
    -device virtio-rng-pci,rng=rng0 \
    -boot menu=on \
    "$@"
