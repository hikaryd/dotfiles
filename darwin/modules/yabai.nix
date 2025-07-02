{ ... }: {
  services.yabai = {
    enable = false;
    enableScriptingAddition = true;
    extraConfig = ''
      source $HOME/.config/yabai/yabairc
    '';
  };
}

