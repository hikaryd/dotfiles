{ ... }: {
  services.yabai = {
    enable = true;
    enableScriptingAddition = true;
    extraConfig = ''
      source $HOME/.config/yabai/yabairc
    '';
  };
}

