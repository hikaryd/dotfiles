# Быстрый Nushell; общие команды выполняются исходным zsh-кодом.
$env.EDITOR = "nvim"
$env.VISUAL = "nvim"
$env.DOTS_ROOT = ($env.DOTS_ROOT? | default ($nu.home-dir | path join "dots"))
$env.DOTS_ZSH_BIN = ($env.DOTS_ZSH_BIN? | default (
  ($env.XDG_CACHE_HOME? | default ($nu.home-dir | path join ".cache"))
  | path join "dots" "zsh-bridge" "bin"
))
$env.PATH = ($env.PATH | prepend [
  $env.DOTS_ZSH_BIN
  ($nu.home-dir | path join ".local/bin")
  ($nu.home-dir | path join ".cargo/bin")
  ($nu.home-dir | path join ".bun/bin")
  "/opt/homebrew/bin" "/opt/homebrew/sbin" "/usr/local/bin"
] | uniq)
$env.config = ($env.config | merge {
  show_banner: false
  table: {mode: light, index_mode: auto, header_on_separator: false, missing_value_symbol: "—"}
  footer_mode: auto
  completions: {case_sensitive: false, quick: true, partial: true}
  history: {max_size: 100000, sync_on_enter: true, file_format: plaintext}
  filesize: {unit: MiB}
})
source ayu.nu
source bridge.nu
source prompt.nu

# Не меняем поведение completion/history, только их стандартные яркие цвета.
$env.config.menus = ($env.config.menus | each {|menu|
  $menu | merge {style: {
    text: "#bfbdb6"
    selected_text: {fg: "#73d0ff", bg: "#273747"}
    description_text: "#858d9c"
    match_text: {fg: "#73d0ff"}
    selected_match_text: {fg: "#73d0ff", bg: "#273747", attr: b}
  }}
})

# Только операции, которым нужно менять состояние родительского Nushell.
alias .. = cd ..
alias ... = cd ../..
alias .... = cd ../../..
alias vs = overlay use .venv/bin/activate.nu
alias nu-open = open
alias open = ^open


$env.config.keybindings ++= [{
  name: dots_menu
  modifier: control
  keycode: char_o
  mode: [emacs vi_normal vi_insert]
  event: {send: executehostcommand, cmd: "dots"}
}]
