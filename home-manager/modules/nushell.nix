{ pkgs, ... }: {
  home.packages = with pkgs; [ zoxide ];

  programs.nushell = {
    enable = true;
    package = pkgs.nushell;

    environmentVariables = {
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

      nu-open = "open";
      open = "^open";
      parse_dir =
        "repomix --ignore '*.lock,docs/*,.git/*,.idea/*,.vscode/*,__pycache__'";
      vs = "overlay use .venv/bin/activate.nu";
      proxy = "mgraftcp --socks5 127.0.0.1:2080";
      md_convert = "pandoc -s -o output.pdf --pdf-engine=typst";

      # Docker
      dc = "docker compose";
      create_mr = "${../../scripts/ai_helper} --mode mr --api gemini";
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
      hms = "nix run nix-darwin -- switch --flake '.#hikary' --impure -v";
      hmc = "nix-collect-garbage -d";
    };

    extraEnv = ''
      mkdir ~/.cache/zoxide
      $env.__zoxide_hooked = true
      ${pkgs.zoxide}/bin/zoxide init nushell | save -f ~/.cache/zoxide/init.nu
    '';

    extraConfig = ''
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

      source ~/.cache/zoxide/init.nu
    '';
  };
}
