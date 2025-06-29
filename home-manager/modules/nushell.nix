{ user, lib, ... }: {
  home.file.".catppuccin_mocha.nu".source = ../configs/nu/catppuccin_mocha.nu;
  programs.nushell = {
    enable = true;

    environmentVariables = {
      EDITOR = "nvim";
      VISUAL = "nvim";
      PATH =
        "/Users/${user}/.nix-profile/bin:/etc/profiles/per-user/${user}/bin:/run/current-system/sw/bin:/nix/var/nix/profiles/default/bin:/usr/local/bin:/opt/homebrew/bin:/Users/${user}/.local/bin:/Users/${user}/.cargo/bin:/Users/${user}/.lmstudio/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/Applications";
      ENV_CONVERSIONS = {
        PATH = {
          from_string =
            lib.hm.nushell.mkNushellInline "{|s| $s | split row (char esep) }";
          to_string =
            lib.hm.nushell.mkNushellInline "{|v| $v | str join (char esep) }";
        };
      };
    };

    shellAliases = {
      v = "nvim";
      cat = "bat --style=plain";
      "." = "cd ..";
      ".." = "cd ../..";
      "..." = "cd ../../..";
      l = "ls";
      cd = "z";
      ccost = "npx ccusage@latest";
      clive = "npx ccusage@latest blocks --live";
      # gemini = "npx https://github.com/google-gemini/gemini-cli";
      # GOOGLE_CLOUD_PROJECT = "653255600806";

      speedtest = "networkquality";

      nu-open = "open";
      open = "^open";
      vs = "overlay use .venv/bin/activate.nu";
      share_port = "npx tunnelmole 8000";

      c = "clear";

      create_mr = "${../../scripts/ai_helper} --mode mr";

      lg = "lazygit";
      gaa = "git ad -A";

      # Nix
      hms = "sudo darwin-rebuild switch --flake '.#hikary' --impure -v";
      hmc = "nix-collect-garbage -d";
    };

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

      source ~/.catppuccin_mocha.nu
    '';
  };
}
