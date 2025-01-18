#!/bin/bash

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

if [ "$EUID" -ne 0 ] && ! sudo -v; then
	echo -e "${RED}Для работы скрипта требуются права sudo${NC}"
	exit 1
fi

sudo -v

log() {
	local level=$1
	local message=$2
	local color

	case $level in
	"INFO") color=$BLUE ;;
	"SUCCESS") color=$GREEN ;;
	"WARNING") color=$YELLOW ;;
	"ERROR") color=$RED ;;
	esac

	echo -e "${color}[$(date '+%Y-%m-%d %H:%M:%S')] [$level] $message${NC}"
}

confirm() {
	local question=$1
	local default=${2:-"n"}

	if [ "$default" = "y" ]; then
		local prompt="[Y/n]"
	else
		local prompt="[y/N]"
	fi

	read -p "$(echo -e ${BLUE}[$(date '+%Y-%m-%d %H:%M:%S')] [QUESTION]${NC} $question $prompt) " answer

	if [ -z "$answer" ]; then
		answer=$default
	fi

	case "$answer" in
	[yY] | [yY][eE][sS]) return 0 ;;
	*) return 1 ;;
	esac
}

check_error() {
	if [ $? -ne 0 ]; then
		log "ERROR" "$1"
		exit 1
	fi
}

check_command() {
	if ! command -v $1 &>/dev/null; then
		log "WARNING" "Команда $1 не найдена"
		return 1
	fi
	return 0
}

setup_capabilities() {
	log "INFO" "Настройка capabilities для $1"
	sudo setcap CAP_NET_ADMIN=ep "$1"
	check_error "Ошибка при установке capabilities для $1"
}

update_system() {
	log "INFO" "Обновление системы..."
	sudo pacman -Syu --noconfirm python
	check_error "Ошибка при обновлении системы"
}

install_paru() {
	log "INFO" "Начало установки paru..."

	if check_command paru; then
		log "INFO" "paru уже установлен"
		return 0
	fi

	log "INFO" "Установка зависимостей для paru..."
	sudo pacman -S --needed --noconfirm git base-devel
	check_error "Ошибка при установке зависимостей paru"

	local temp_dir=$(mktemp -d)
	cd "$temp_dir"

	git clone https://aur.archlinux.org/paru-bin.git
	check_error "Ошибка при клонировании репозитория paru"

	cd paru-bin
	makepkg -si --noconfirm
	check_error "Ошибка при установке paru"

	cd
	rm -rf "$temp_dir"

	log "SUCCESS" "paru успешно установлен"
}

install_amd_drivers() {
	if confirm "Хотите установить драйверы AMD?"; then
		log "INFO" "Установка драйверов AMD..."

		sudo pacman -S --needed --noconfirm \
			mesa \
			lib32-mesa \
			xf86-video-amdgpu \
			vulkan-radeon \
			lib32-vulkan-radeon \
			vulkan-icd-loader \
			lib32-vulkan-icd-loader \
			libva-mesa-driver \
			lib32-libva-mesa-driver
		check_error "Ошибка при установке основных драйверов AMD"

		if confirm "Установить дополнительные утилиты для AMD (CoreCtrl, radeontop)?"; then
			if check_command paru; then
				paru -S --needed --noconfirm corectrl radeontop
				check_error "Ошибка при установке дополнительных утилит AMD"
			else
				log "WARNING" "paru не установлен, пропуск установки дополнительных утилит"
			fi
		fi

		log "SUCCESS" "Драйверы AMD успешно установлены"
	else
		log "INFO" "Пропуск установки драйверов AMD"
	fi
}

install_nix() {
	log "INFO" "Начало установки Nix..."

	if [ -d "/nix" ]; then
		log "INFO" "Nix уже установлен"
		return 0
	fi

	curl --proto '=https' --tlsv1.2 -sSfL https://nixos.org/nix/install -o nix-install.sh
	check_error "Ошибка при загрузке установочного скрипта Nix"

	chmod +x nix-install.sh
	./nix-install.sh --daemon
	check_error "Ошибка при установке Nix"

	rm nix-install.sh

	sudo systemctl enable --now nix-daemon.service
	check_error "Ошибка при активации службы nix-daemon"

	sudo usermod -aG nix-users $USER
	check_error "Ошибка при добавлении пользователя в группу nix-users"

	if [ -f "/etc/profile.d/nix.sh" ]; then
		. /etc/profile.d/nix.sh
	else
		log "ERROR" "Файл nix.sh не найден"
		exit 1
	fi

	nix-channel --add https://nixos.org/channels/nixpkgs-unstable
	nix-channel --update
	check_error "Ошибка при настройке каналов Nix"

	echo "max-jobs = auto" | sudo tee -a /etc/nix/nix.conf
	echo "experimental-features = nix-command flakes" | sudo tee -a /etc/nix/nix.conf
	check_error "Ошибка при настройке max-jobs"

	log "SUCCESS" "Nix успешно установлен"
}

install_home_manager() {
	log "INFO" "Установка home-manager..."

	if ! command -v nix-channel &>/dev/null; then
		log "ERROR" "Nix не установлен. Установите сначала Nix"
		exit 1
	fi

	nix-channel --add https://github.com/nix-community/home-manager/archive/master.tar.gz home-manager
	nix-channel --update
	check_error "Ошибка при добавлении канала home-manager"

	export NIX_PATH=$HOME/.nix-defexpr/channels:/nix/var/nix/profiles/per-user/root/channels${NIX_PATH:+:$NIX_PATH}
	nix-shell '<home-manager>' -A install
	check_error "Ошибка при установке home-manager"

	log "SUCCESS" "home-manager успешно установлен"
}

install_emptty() {
	log "INFO" "Установка emptty..."

	if check_command pacman; then
		sudo pacman -S --needed --noconfirm emptty
		check_error "Ошибка при установке emptty через pacman"

		sudo systemctl enable emptty
		check_error "Ошибка при активации службы emptty"

		log "SUCCESS" "emptty успешно установлен"
	else
		log "WARNING" "Pacman не найден, пропуск установки emptty"
	fi
}

install_docker() {
	log "INFO" "Установка docker..."

	if check_command pacman; then
		sudo pacman -S --needed --noconfirm docker
		check_error "Ошибка при установке docker через pacman"

		sudo usermod -aG docker $USER
		sudo systemctl enable --now docker
		check_error "Ошибка при активации службы docker"

		log "SUCCESS" "docker успешно установлен"
	else
		log "WARNING" "Pacman не найден, пропуск установки docker"
	fi
}

main() {
	log "INFO" "Начало установки и настройки системы..."

	sudo -v

	update_system

	install_paru
	install_amd_drivers
	install_nix
	install_docker
	install_home_manager
	install_emptty

	if [ -f "/home/$USER/.config/nekoray/config/vpn-run-root.sh" ]; then
		setup_capabilities "/home/$USER/.config/nekoray/config/vpn-run-root.sh"
	else
		log "WARNING" "Скрипт vpn-run-root.sh не найден"
	fi

	log "SUCCESS" "Установка и настройка завершены успешно!"
	log "INFO" "Пожалуйста, перезагрузите систему для применения всех изменений"
	log "WARNING" "Некоторые изменения (например, добавление в группы) вступят в силу только после перезагрузки"
}

main
