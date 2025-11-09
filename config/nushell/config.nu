# Переменные окружения
$env.EDITOR = "nvim"
$env.VISUAL = "nvim"

# Пути
$env.PATH = (
  $env.PATH
  | split row (char esep)
  | prepend "/usr/local/bin"
  | prepend "/opt/homebrew/bin"
  | prepend "~/.local/bin"
  | prepend "~/.cargo/bin"
  | append "/Applications"
)

# Алиасы
alias v = nvim
alias cat = bat --style=plain
alias "." = cd ..
alias ".." = cd ../..
alias "..." = cd ../../..
alias l = ls
alias bu = brew upgrade --cask --greedy
# alias cd = z

alias speedtest = networkquality
alias codex = codex -a untrusted -c model_reasoning_effort="high"

alias nu-open = open
alias open = ^open
alias vs = overlay use .venv/bin/activate.nu
alias share_port = npx tunnelmole 8000

alias c = clear

alias create_mr = ~/dots/scripts/ai_helper --mode mr

alias lg = lazygit
alias gaa = git add -A
alias nvim-bench = hyperfine "nvim --startuptime /tmp/startup.log +qall" --warmup 3 --runs 10

$env.config = {
  show_banner: false
  table: {
    mode: "rounded"
    index_mode: "always"
  }
  completions: {
    case_sensitive: false
    quick: true
    partial: true
  }
  history: {
    max_size: 100000
    sync_on_enter: true
    file_format: "plaintext"
  }
  filesize: {
    unit: "MiB"
  }
}

# Тема
source ~/.config/nushell/catppuccin_mocha.nu

# Пользовательские команды
def extract [file: string] {
  if ($file | is-empty) {
    echo "Usage: extract <file>"
    return 1
  }
  if (not ($file | path exists)) {
    echo $"'($file)' is not a valid file"
    return 1
  }
  let ext = ($file | split row '.' | last)
  match $ext {
    "tar"  => { run-external "tar" "xvf" $file }
    "tgz"  => { run-external "tar" "xvzf" $file }
    "tbz2" => { run-external "tar" "xvjf" $file }
    "bz2"  => { run-external "bunzip2" $file }
    "gz"   => { run-external "gunzip" $file }
    "zip"  => { run-external "unzip" $file }
    "rar"  => { run-external "unrar" "x" $file }
    "7z"   => { run-external "7z" "x" $file }
    "xz"   => { run-external "xz" "--decompress" $file }
    "Z"    => { run-external "uncompress" $file }
    _      => { echo "unsupported file extension"; return 1 }
  }
}


mkdir ($nu.data-dir | path join "vendor/autoload")
starship init nu | save -f ($nu.data-dir | path join "vendor/autoload/starship.nu")

zoxide init nushell | save -f ~/.zoxide.nu
source ~/.zoxide.nu
source $"($nu.home-path)/.cargo/env.nu"
