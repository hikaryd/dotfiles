{ pkgs, ... }: {
  home.packages = with pkgs; [ kanata ];
  xdg.configFile."kanata/kanata.kbd".text = ''
    (defcfg
      process-unmapped-keys yes
    )
    (defsrc
      ⎋    f1   f2   f3   f4   f5   f6   f7   f8   f9   f10  f11  f12
      caps a    s    d    f    g    h    j    k    l
    )

    (defvar
      tap-timeout 100
      hold-timeout 200

      tt $tap-timeout
      ht $hold-timeout
    )

    (deftemplate triple-tap-layer-switch (key layer-name)
      $key (tap-dance 200 (
        $key
        (macro $key $key)
        (layer-switch $layer-name)
      ))
    )

    (deflayermap (default)
      s (tap-hold-release $tt $ht s lmet)
      f (tap-hold-release $tt $ht f rctl)

      spc (tap-hold $tt $ht spc (layer-while-held extended))
    )

    (deflayermap (extended)
      k up
      h left
      j down
      l rght
    )
    (deflayer base
      ⎋    f1   f2   f3   f4   f5   f6   f7   f8   f9   f10  f11  f12
      f13 a    s    d    f    g    h    j    k    l
    )
  '';
}
