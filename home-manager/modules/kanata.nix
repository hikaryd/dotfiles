{ ... }: {

  xdg.configFile."kanata/kanata.kbd".text = ''
    (defsrc
      esc  f1   f2   f3   f4   f5   f6   f7   f8   f9   f10  f11  f12
      grv  1    2    3    4    5    6    7    8    9    0    -    =   bspc
      tab  q    w    e    r    t    y    u    i    o    p    [    ]   \
      caps a    s    d    f    g    h    j    k    l    ;    '    ret
      lsft z    x    c    v    b    n    m    ,    .    /    rsft
      lctl lmet lalt           spc            ralt rmet cmp  rctl
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
      caps (tap-hold $tt $ht caps lctl)

      (template-expand triple-tap-layer-switch grv transparent)
      spc (tap-hold $tt $ht spc (layer-while-held extended))
    )


    (deflayermap (transparent)
      (template-expand triple-tap-layer-switch grv default)
    )

    (deflayermap (extended)
      u pgdn
      o pgup

      i up
      j left
      k down
      l rght

      h C-left
      ; C-rght

      m C-S-tab ;; previous tab
      , C-tab ;; next tab

      n home
      . end

      r C-. ;; paste
      t C-i ;; yank/copy
      f C-/ ;; undo
      b C-b ;; cut
    )
  '';
}
