# Аргументы идут отдельными argv; приватные определения остаются в private.zsh.
alias zs = ^($env.DOTS_ROOT | path join "scripts/dots-zsh") --run

# Только навигация: TUI получает настоящий терминал, не буферизированный pipeline.
def --env --wrapped dots-cd [name: string, ...args: string] {
  let target = (^mktemp -t dots-nu-cwd | str trim)
  try {
    try {
      ^($env.DOTS_ROOT | path join "scripts/dots-zsh") --cwd-file $target --run $name ...$args
    } catch { }
    let code = $env.LAST_EXIT_CODE
    let directory = (open --raw $target)
    rm $target
    if ($directory | is-not-empty) { cd $"($directory)/." }
    $env.LAST_EXIT_CODE = $code
    if $code != 0 { ^/bin/sh -c 'exit "$1"' -- ($code | into string) }
  } catch {|err|
    rm -f $target
    error make $err
  }
}
def --env --wrapped z [...args: string] { dots-cd z ...$args }
def --env --wrapped zi [...args: string] { dots-cd zi ...$args }
def --env --wrapped y [...args: string] { dots-cd y ...$args }
def --env --wrapped tp [...args: string] { dots-cd tp ...$args }
