{ pkgs, lib, ... }: {
  programs.carapace.enable = true;
  programs.carapace.enableNushellIntegration = true;
  home.activation = {
    addNushellToEtcShells = lib.hm.dag.entryAfter [ "writeBoundary" ] ''
      if [ -f /etc/shells ]; then
        nushell_path="$HOME/.nix-profile/bin/nu"
        if ! grep -q "$nushell_path" /etc/shells; then
          echo "Adding $nushell_path to /etc/shells"
          echo "$nushell_path" | sudo tee -a /etc/shells
        fi
      fi
    '';
  };
  programs.nushell = {
    enable = true;
    package = pkgs.nushell;

    environmentVariables = {
      # XDG
      DOCKER_CONFIG = "$XDG_CONFIG_HOME/docker";
      PYTHONSTARTUP = "$XDG_CONFIG_HOME/python/pythonrc";
      CARGO_HOME = "$XDG_DATA_HOME/cargo";
      LESSHISTFILE = "$XDG_STATE_HOME/less/history";

      # Локали
      LANG = "en_US.UTF-8";
      LC_CTYPE = "ru_RU.UTF-8";

      # Основные
      EDITOR = "nvim";
      VISUAL = "nvim";

      TERM = "tmux-256color";
    };

    shellAliases = {
      v = "nvim";
      s = "sudo";
      ".." = "cd ..";
      l = "ls";
      tree = "eza --tree --level=2 --icons";
      "csv-view" = "csvlens";

      parse_dir =
        "repomix --ignore '*.lock,docs/*,.git/*,.idea/*,.vscode/*,__pycache__'";

      # Docker
      dc = "docker compose";
      dcl = "docker compose logs -f";
      dcub = "docker compose up --build -d --force-recreate";

      # Python
      # vs = "source .venv/bin/activate";

      # Git
      lg = "lazygit";

      # Nix
      hms = "home-manager switch --flake '.#hikary'";
      hmb = "home-manager build --flake '.#hikary'";
      hmc =
        "do { nix-collect-garbage -d; home-manager expire-generations '-30 days' }";
    };

    extraConfig = ''
      # Базовые функции
      def extract [file: string] {
        if ($file | is-empty) {
          echo "Usage: extract <file>"
          return 1
        }
        
        if (not ($file | path exists)) {
          echo "'$file' is not a valid file"
          return 1
        }
        
        let ext = ($file | split row '.' | last)
        match $ext {
          "tar" => { run-external "tar" "xvf" $file }
          "tgz" => { run-external "tar" "xvzf" $file }
          "tbz2" => { run-external "tar" "xvjf" $file }
          "bz2" => { run-external "bunzip2" $file }
          "gz" => { run-external "gunzip" $file }
          "zip" => { run-external "unzip" $file }
          "rar" => { run-external "unrar" "x" $file }
          "7z" => { run-external "7z" "x" $file }
          "xz" => { run-external "xz" "--decompress" $file }
          "Z" => { run-external "uncompress" $file }
          _ => { echo "unsupported file extension"; return 1 }
        }
      }

      def detect-project-type [] {
        if ("package.json" | path exists) {
          "node"
        } else if (["requirements.txt" "Pipfile" "pyproject.toml"] | any { |it| $it | path exists }) {
          "python"
        } else if (["docker-compose.yml" "Dockerfile"] | any { |it| $it | path exists }) {
          "docker"
        } else {
          "default"
        }
      }

      # Git функции
      def git-cleanup [] {
        let branches = (run-external "git" "branch" "--merged" "main" 
          | lines 
          | where { |it| not ($it | str contains "* main") }
        )
        if not ($branches | is-empty) {
          $branches | each { |branch| run-external "git" "branch" "-d" ($branch | str trim) }
        }
      }

      def git-recent [] {
        run-external "git" "for-each-ref" "--sort=-committerdate" "refs/heads/" "--format=%(refname:short)" 
        | lines 
        | first 5
      }

      def git-contrib [] {
        run-external "git" "shortlog" "-sn" "--all" "--no-merges"
      }

      # Docker утилиты
      def docker-cleanup [] {
        run-external "docker" "system" "prune" "-af"
        run-external "docker" "volume" "prune" "-f"
      }

      def docker-stop-all [] {
        let containers = (run-external "docker" "ps" "-q" | lines)
        if not ($containers | is-empty) {
          $containers | each { |id| run-external "docker" "stop" $id }
        }
      }

      # Конфигурация окружения
      $env.config = {
        show_banner: false
        table: {
          mode: rounded
          index_mode: always
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
          metric: true
          format: "auto"
        }
      }
    '';
  };

  home.packages = with pkgs; [ nu_scripts ];
}
