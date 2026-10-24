# Quiet Ayu: тот же двухстрочный prompt, что и в zsh, без Starship.
def _dots-git-dir [cwd: string] {
  mut dir = $cwd
  loop {
    let marker = ($dir | path join ".git")
    if ($marker | path type) == dir { return $marker }
    if ($marker | path type) == file {
      let line = (try { open --raw $marker | lines | first } catch { "" })
      if ($line | str starts-with "gitdir: ") {
        let target = ($line | str substring 8.. | str trim)
        return (if ($target | str starts-with "/") {
          $target | path expand
        } else {
          $dir | path join $target | path expand --no-symlink
        })
      }
      return ""
    }
    let parent = ($dir | path dirname)
    if $parent == $dir { return "" }
    $dir = $parent
  }
}

def _dots-prompt-text [text: string] {
  # Имена веток/каталогов не должны внедрять управляющие коды в терминал.
  $text | str replace --all --regex '[\x00-\x1f\x7f-\x9f]' ''
}

def --env _dots-refresh-prompt [] {
  if $env.__DOTS_NU_PROMPT.pwd != $env.PWD {
    $env.__DOTS_NU_PROMPT = {pwd: $env.PWD, git_dir: (_dots-git-dir $env.PWD)}
  }
}

# Record не экспортируется дочерним процессам; hook не дублируется при source.
if '__DOTS_NU_PROMPT' not-in $env {
  $env.__DOTS_NU_PROMPT = {pwd: "", git_dir: ""}
  $env.config.hooks.pre_prompt ++= [{|| _dots-refresh-prompt }]
}
_dots-refresh-prompt

$env.PROMPT_COMMAND = {||
  let dim = (ansi '#858d9c')
  let text = (ansi '#bfbdb6')
  let reset = (ansi reset)
  let style_file = (($env.XDG_CONFIG_HOME? | default ($nu.home-dir | path join ".config")) | path join "dots/style")
  let presentation = (try { (open --raw $style_file | str trim) == "presentation" } catch { false })
  let elapsed = (try { $env.CMD_DURATION_MS | into int } catch { 0 })
  let duration = if $elapsed >= 3000 { $" ($dim)(($elapsed / 1000) | math round)s($reset)" } else { "" }
  if $presentation {
    $"($text)PRESENTATION($reset)($duration)\n"
  } else {
    let relative = (try { $env.PWD | path relative-to $nu.home-dir } catch { null })
    let path = if $relative == null { $env.PWD } else if $relative == "" { "~" } else { $"~/($relative)" }
    let parts = ($path | split row '/')
    let directory = if ($parts | length) > 5 { $"…/($parts | last 5 | str join '/')" } else { $path }
    let head = if $env.__DOTS_NU_PROMPT.git_dir == "" { "" } else {
      try { open --raw ($env.__DOTS_NU_PROMPT.git_dir | path join HEAD) | lines | first } catch { "" }
    }
    let branch = if ($head | str starts-with 'ref: refs/heads/') {
      $head | str substring 16..
    } else { $head | str substring 0..<7 }
    let git = if $branch == "" { "" } else { $" ($dim)(_dots-prompt-text $branch)($reset)" }
    let venv = ($env.VIRTUAL_ENV? | default "" | path basename)
    let python = if $venv == "" { "" } else { $" (ansi '#95e6cb')(_dots-prompt-text $venv)($reset)" }
    $"($dim)󰀵 ($text)(_dots-prompt-text $directory)($reset)($git)($python)($duration)\n"
  }
}
$env.PROMPT_COMMAND_RIGHT = {|| "" }
$env.PROMPT_INDICATOR = {||
  if ($env.LAST_EXIT_CODE? | default 0) != 0 { $"(ansi '#f07178')!(ansi reset) " } else { $"(ansi '#73d0ff')>(ansi reset) " }
}
$env.PROMPT_INDICATOR_VI_INSERT = $env.PROMPT_INDICATOR
$env.PROMPT_INDICATOR_VI_NORMAL = $"(ansi '#858d9c')<(ansi reset) "
$env.PROMPT_MULTILINE_INDICATOR = $"(ansi '#858d9c')· (ansi reset)"
