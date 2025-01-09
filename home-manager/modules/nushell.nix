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
      LESSHISTFILE = "$XDG_STATE_HOME/less/history";

      # Локали
      LANG = "en_US.UTF-8";
      LC_CTYPE = "ru_RU.UTF-8";

      # Основные
      EDITOR = "nvim";
      VISUAL = "nvim";
    };

    shellAliases = {
      v = "nvim";
      ssh = ''env TERM="xterm-256color" ssh'';
      cat = "bat --style=plain";
      ".." = "cd ..";
      l = "ls";
      cd = "z";
      parse_dir =
        "repomix --ignore '*.lock,docs/*,.git/*,.idea/*,.vscode/*,__pycache__'";
      vs = "overlay use .venv/bin/activate.nu";

      # Docker
      dc = "docker compose";
      osu = ''env WLR_DRM_NO_ATOMIC="1" osu'';
      dcl = "docker compose logs -f";
      dcub = "docker compose up --build -d --force-recreate";
      dcd = "docker compose down";
      dcr = "docker compose restart";
      dps = "docker ps";
      dpsa = "docker ps -a";
      di = "docker images";
      dprune = "docker system prune -af";
      dcp = "docker container prune -f";
      dip = "docker image prune -af";
      dvp = "docker volume prune -f";

      # Git
      lg = "lazygit";
      gs = "git status";
      ga = "git add";
      gaa = "git add --all";
      gc = "git commit -m";
      gp = "git push";
      gpl = "git pull";
      gf = "git fetch --all";
      gb = "git branch";
      gco = "git checkout";
      gcb = "git checkout -b";
      gd = "git diff";
      gl = "git log --oneline";
      grs = "git restore --staged";
      grh = "git reset --hard";
      gst = "git stash";
      gstp = "git stash pop";
      gm = "git merge";
      grb = "git rebase";
      gcp = "git cherry-pick";
      gtl = "git tag -l";
      gta = "git tag -a";

      # Nix
      hms = "home-manager switch --flake '.#hikary' --impure";
      hmb = "home-manager build --flake '.#hikary'";
      hmc =
        "do { nix-collect-garbage -d; home-manager expire-generations '-30 days' }";
    };

    extraEnv = ''
      mkdir ~/.cache/zoxide
      zoxide init nushell | str replace --all "-- $rest" "-- ...$rest" | str replace --all "def-env" "def --env" | save -f ~/.cache/zoxide/init.nu
    '';

    extraConfig = ''
          $env.OPENROUTER_API_KEY = (open ($env.HOME + '/creds/open_router') | str trim)
          $env.SSH_AUTH_SOCK = ($env.XDG_RUNTIME_DIR | path join "ssh-agent.socket")
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
          source ~/.cache/zoxide/init.nu
    '';
  };

  home.packages = with pkgs; [ nu_scripts zoxide ];
}
