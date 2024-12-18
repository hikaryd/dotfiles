{ config, ... }: {
  xdg.configFile."hypr/hyprpaper.conf".text = ''
    preload = ${config.home.homeDirectory}/dotfiles/wallpapers/minimalist-moon-night-mountains.jpg
    wallpaper = ,${config.home.homeDirectory}/dotfiles/wallpapers/minimalist-moon-night-mountains.jpg
    ipc = off
  '';
}
