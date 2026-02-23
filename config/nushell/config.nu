# Переменные окружения
$env.EDITOR = "nvim"
$env.VISUAL = "nvim"
# $env.config.edit_mode = "vi"

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
alias fg = froggit
alias bu = brew upgrade --cask --greedy
alias deploy-dev = ~/dots/scripts/deploy-dev.sh

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
source $"($nu.home-dir)/.cargo/env.nu"

# Kafka consumer через kcat с SSL
def kafka-consume [
  creds_file: path,    # Путь к JSON файлу с кредами
  brokers: string,     # Kafka брокеры (host:port или host1:port1,host2:port2)
  topic: string,       # Топик для чтения
  --from-beginning (-b) # Читать с начала топика
  --group (-g): string  # Consumer group
  --json (-j)          # JSON вывод
  --skip-metadata (-s) # Пропустить запрос метаданных
] {
  # Читаем креды
  let creds = (nu-open $creds_file)

  # Временная директория для сертификатов
  let temp_dir = (mktemp -d | str trim)
  let ca_file = $"($temp_dir)/ca.pem"
  let cert_file = $"($temp_dir)/cert.pem"
  let key_file = $"($temp_dir)/key.pem"

  # Записываем сертификаты
  ($creds | get "ca.pem") | save -f $ca_file
  ($creds | get "cert.pem") | save -f $cert_file
  ($creds | get "key.pem") | save -f $key_file

  # SSL аргументы
  let ssl_args = [
    -X $"security.protocol=ssl"
    -X $"ssl.ca.location=($ca_file)"
    -X $"ssl.certificate.location=($cert_file)"
    -X $"ssl.key.location=($key_file)"
  ]

  # Пароль ключа
  let key_passwd = ($creds | get -o "kafka.keystore.keypasswd" | default "")
  mut ssl_args_full = $ssl_args
  if ($key_passwd | is-not-empty) {
    $ssl_args_full = ($ssl_args_full | append [-X $"ssl.key.password=($key_passwd)"])
  }

  # Запрос метаданных перед потреблением
  if not $skip_metadata {
    print "📊 Получение метаданных топика..."
    print ""
    ^kcat -b $brokers -t $topic -L ...$ssl_args_full
    print ""
    print "─────────────────────────────────────"
    print "🚀 Начинаем потребление сообщений..."
    print ""
  }

  # Базовые аргументы для потребления
  mut args = [
    -b $brokers
    -t $topic
    -C
    ...$ssl_args_full
  ]

  # Читать с начала
  if $from_beginning {
    $args = ($args | append [-o beginning])
  }

  # Consumer group
  if $group != null {
    $args = ($args | append [-G $group $topic])
  }

  # JSON формат
  if $json {
    $args = ($args | append [-J])
  }

  # Запуск kcat
  ^kcat ...$args

  # Cleanup
  rm -rf $temp_dir
}

# Kafka producer через kcat с SSL
def kafka-produce [
  creds_file: path,    # Путь к JSON файлу с кредами
  brokers: string,     # Kafka брокеры (host:port или host1:port1,host2:port2)
  topic: string,       # Топик для записи
  message?: string     # Сообщение (если не указано, читает из stdin)
] {
  # Читаем креды
  let creds = (nu-open $creds_file)

  # Временная директория для сертификатов
  let temp_dir = (mktemp -d | str trim)
  let ca_file = $"($temp_dir)/ca.pem"
  let cert_file = $"($temp_dir)/cert.pem"
  let key_file = $"($temp_dir)/key.pem"

  # Записываем сертификаты
  ($creds | get "ca.pem") | save -f $ca_file
  ($creds | get "cert.pem") | save -f $cert_file
  ($creds | get "key.pem") | save -f $key_file

  # SSL аргументы
  mut args = [
    -b $brokers
    -t $topic
    -P
    -X $"security.protocol=ssl"
    -X $"ssl.ca.location=($ca_file)"
    -X $"ssl.certificate.location=($cert_file)"
    -X $"ssl.key.location=($key_file)"
  ]

  # Пароль ключа
  let key_passwd = ($creds | get -o "kafka.keystore.keypasswd" | default "")
  if ($key_passwd | is-not-empty) {
    $args = ($args | append [-X $"ssl.key.password=($key_passwd)"])
  }

  # Запуск kcat
  if $message != null {
    $message | ^kcat ...$args
    print $"✅ Сообщение отправлено в топик ($topic)"
  } else {
    print "📝 Введите сообщения (Ctrl+D для завершения):"
    ^kcat ...$args
  }

  # Cleanup
  rm -rf $temp_dir
}
