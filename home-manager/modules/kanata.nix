{ pkgs, ... }:
let
  isDarwin = pkgs.stdenv.isDarwin;

  defsrc = if !isDarwin then ''
    (defsrc
          esc  f1   f2   f3   f4   f5   f6   f7   f8   f9   f10  f11  f12
          grv  1    2    3    4    5    6    7    8    9    0    -    =   bspc
          tab  q    w    e    r    t    y    u    i    o    p    [    ]   \
          caps a    s    d    f    g    h    j    k    l    ;    '    ret
          lsft z    x    c    v    b    n    m    ,    .    /    rsft
          lctl lmet lalt           spc            ralt rmet cmp  rctl
        )

    	'' else ''
      (deflocalkeys-macos
        ß    12
        ´    13
        z    21
        ü    26
        +    27
        ö    39
        ä    40
        <    41
        #    43
        y    44
        -    53
        ^    86
      )

      (defsrc
        ⎋         f1   f2   f3   f4   f5   f6   f7   f8   f9   f10  f11  f12
        `         1    2    3    4    5    6    7    8    9    0    ß    ´    ⌫
        ↹         q    w    e    r    t    z    u    i    o    p    ü    +
        ⇪         a    s    d    f    g    h    j    k    l    ö    ä    #    ↩
       ‹⇧   <     y    x    c    v    b    n    m    ,    .    -         ▲    ⇧›
        fn       ‹⌃   ‹⌥   ‹⌘              ␣              ⌘›   ⌥›   ◀    ▼    ▶
      )
    '';

in {
  home.packages = with pkgs; [ kanata ];
  xdg.configFile."kanata/kanata.kbd".text = ''
    ${defsrc}

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
      ;; a (tap-hold-release $tt $ht a lalt)
      s (tap-hold-release $tt $ht s lmet)
      ;; d (tap-hold-release $tt $ht d lsft)
      f (tap-hold-release $tt $ht f lctl)
      ;; lctrl  (tap-hold-release $tt $ht esc lctrl)
      fn ‹⌃ 

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
      v (macro C-a 10 v)
      c (macro C-a 10 C-c)
      x (macro C-a 10 c)
      s (macro C-a 10 S-t)
      a C-a
      lctrl esc

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

      t C-S-v ;; paste
      r C-c ;; yank/copy
      f C-z ;; undo
      b C-x ;; cut
    )
  '';
}
