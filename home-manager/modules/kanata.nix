{ pkgs, ... }: {
  home.packages = with pkgs; [ kanata ];
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
      hold-timeout 150

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
      a (tap-hold-release $tt $ht a lalt)
      s (tap-hold-release $tt $ht s lmet)
      d (tap-hold-release $tt $ht d lsft)
      f (tap-hold-release $tt $ht f lctl)
      lctrl  (tap-hold-release $tt $ht esc lctrl)

      spc (tap-hold $tt $ht spc (layer-while-held extended))
    )


    (deflayermap (extended)
      1 (macro C-a 10 Digit1)
      2 (macro C-a 10 Digit2)
      3 (macro C-a 10 Digit3)
      4 (macro C-a 10 Digit4)
      5 (macro C-a 10 Digit5)
      6 (macro C-a 10 Digit6)
      7 (macro C-a 10 Digit7)
      8 (macro C-a 10 Digit8)

      u pgdn
      o pgup
      w C-bspc

      k up
      h left
      j down
      l rght

      g C-left
      ; C-rght

      m C-S-tab ;; previous tab
      , C-tab ;; next tab
      q C-S-p ;; task groups

      n home
      . end

      r C-S-v ;; paste
      t C-c ;; yank/copy
      f C-z ;; undo
      b C-x ;; cut
    )
  '';
}
