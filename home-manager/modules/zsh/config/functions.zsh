#!/bin/zsh
# make mysql actually usable
detect-project-type() {
    if [ -f "package.json" ]; then
        echo "node"
    elif [ -f "requirements.txt" ] || [ -f "Pipfile" ] || [ -f "pyproject.toml" ]; then
        echo "python"
    elif [ -f "docker-compose.yml" ] || [ -f "Dockerfile" ]; then
        echo "docker"
    else
        echo "default"
    fi
}

hikary-update-all() {
    sudo reflector --verbose --country 'Russia' -l 25 --sort rate --save /etc/pacman.d/mirrorlist
    sudo pacman -Syu --noconfirm
    paru -Syu --noconfirm
}


extract() {
    if [ -z "$1" ]; then
        echo "Usage: extract <file>"
        return 1
    fi

    local file="$1"
    if [ ! -f "$file" ]; then
        echo "'$file' is not a valid file"
        return 1
    fi

    case "$file" in
        *.tar.bz2|*.tbz|*.tbz2) tar xvjf "$file" ;;
        *.tar.gz|*.tgz) tar xvzf "$file" ;;
        *.tar.xz|*.txz) tar xvf "$file" ;;
        *.tar.Z) tar xvZf "$file" ;;
        *.bz2) bunzip2 "$file" ;;
        *.deb) ar x "$file" ;;
        *.gz) gunzip "$file" ;;
        *.pkg) pkgutil --expand "$file" ;;
        *.rar) unrar x "$file" ;;
        *.tar) tar xvf "$file" ;;
        *.xz) xz --decompress "$file" ;;
        *.zip|*.war|*.jar|*.nupkg) unzip "$file" ;;
        *.Z) uncompress "$file" ;;
        *.7z) 7za x "$file" ;;
        *) echo "unsupported file extension"; return 1 ;;
    esac
}
