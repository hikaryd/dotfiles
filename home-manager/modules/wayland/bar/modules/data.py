import json
import os

USERNAME = os.getlogin()
HOSTNAME = os.uname().nodename
HOME_DIR = os.path.expanduser('~')

default_wallpapers_dir = os.path.expanduser(
    '~/dotfiles/home-manager/modules/wayland/bar/assets/wallpapers_example'
)
json_config_path = os.path.expanduser(
    '~/dotfiles/home-manager/modules/wayland/bar/config/config.json'
)

if os.path.exists(json_config_path):
    with open(json_config_path, 'r') as f:
        config = json.load(f)
    WALLPAPERS_DIR = config.get('wallpapers_dir', default_wallpapers_dir)
else:
    WALLPAPERS_DIR = default_wallpapers_dir
