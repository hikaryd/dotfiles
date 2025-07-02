{ ... }: {
  services.sketchybar = {
    enable = true;
    enableScriptingAddition = true;
    extraConfig = ''
      source $HOME/.config/sketchybar/sketchybarrc
    '';
  };
}

