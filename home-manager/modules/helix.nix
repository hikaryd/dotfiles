{ pkgs, ... }:
let isDarwin = pkgs.stdenv.isDarwin;
in {
  programs.helix = {
    enable = true;
    defaultEditor = true;

    extraPackages = with pkgs; [
      ripgrep
      fd
      # JS/TS
      nodePackages.typescript-language-server
      typescript
      nodePackages.vscode-langservers-extracted # html/css/json
      nodePackages.yaml-language-server
      prettierd
      # Python
      pyright
      ruff
      # Rust / Go
      rust-analyzer
      rustfmt
      gopls
      go
      # Nix / Lua / Bash / Markdown / SQL
      nixd
      nixfmt-rfc-style
      lua-language-server
      stylua
      bash-language-server
      shfmt
      marksman
      pgformatter
    ];

    settings = {
      theme = "catppuccin_mocha_transparent";

      editor = {
        line-number = "relative";
        cursorline = true;

        bufferline = "never"; # "always" | "multiple" | "never"

        color-modes = true;
        auto-format = true;

        clipboard-provider = if isDarwin then "pasteboard" else "wayland";

        lsp = {
          display-messages = true;
          display-inlay-hints = false;
          auto-signature-help = true;
          snippets = true;
        };

        statusline = {
          left = [ "mode" "spinner" "file-name" ];
          center = [ ];
          right = [
            "version-control"
            "diagnostics"
            "selections"
            "position"
            "file-encoding"
            "file-line-ending"
            "file-type"
          ];
          separator = "│";
          mode = {
            normal = "NORMAL";
            insert = "INSERT";
            select = "SELECT";
          };
        };

        file-picker = { hidden = false; };

        auto-save = {
          focus-lost = true;
          after-delay = {
            enable = true;
            timeout = 2000;
          };
        };
      };

      keys = {
        normal = {
          "C-s" = ":write";
          q = ":buffer-close";
          "%" = "match_brackets";

          space = {
            e = "file_explorer";

            "?" = "command_palette";
            f = "file_picker";
            F = "file_picker_in_current_directory";
            b = "buffer_picker";
            "/" = "global_search";

            k = "hover";
            r = "rename_symbol";
            a = "code_action";
            d = "workspace_diagnostics_picker";
            s = "symbol_picker";
            S = "workspace_symbol_picker";

            o = "goto_last_accessed_file";
          };

          g = {
            d = "goto_definition";
            i = "goto_implementation";
            r = "goto_reference";
            y = "goto_type_definition";

            o = "goto_last_accessed_file";
            n = "goto_next_buffer";
          };

          esc = [ "collapse_selection" "keep_primary_selection" ];
        };
      };
    };

    languages = {
      language-server = {
        pyright = {
          command = "pyright-langserver";
          args = [ "--stdio" ];
        };
        ruff = {
          command = "ruff";
          args = [ "server" ];
        };
        ty = {
          command = "ty";
          args = [ "server" ];
        };
        tsserver = {
          command = "typescript-language-server";
          args = [ "--stdio" ];
        };
        rust-analyzer = { command = "rust-analyzer"; };
        gopls = { command = "gopls"; };
        nixd = { command = "nixd"; };
        lua = { command = "lua-language-server"; };
        bash = {
          command = "bash-language-server";
          args = [ "start" ];
        };
        marksman = {
          command = "marksman";
          args = [ "server" ];
        };
        yaml = {
          command = "yaml-language-server";
          args = [ "--stdio" ];
        };
        vscode-json = {
          command = "vscode-json-languageserver";
          args = [ "--stdio" ];
        };
      };

      language = [
        # Python
        {
          name = "python";
          language-servers = [ "pyright" "ruff" "ty" ];
          formatter = {
            command = "uvx";
            args = [ "ruff" "format" "-" ];
          };
          auto-format = true;
        }

        # TypeScript/JavaScript/JSON/HTML/CSS
        {
          name = "typescript";
          language-servers = [ "tsserver" ];
          formatter = {
            command = "prettierd";
            args = [ "%file" ];
          };
          auto-format = true;
        }
        {
          name = "tsx";
          language-servers = [ "tsserver" ];
          formatter = {
            command = "prettierd";
            args = [ "%file" ];
          };
          auto-format = true;
        }
        {
          name = "javascript";
          language-servers = [ "tsserver" ];
          formatter = {
            command = "prettierd";
            args = [ "%file" ];
          };
          auto-format = true;
        }
        {
          name = "jsx";
          language-servers = [ "tsserver" ];
          formatter = {
            command = "prettierd";
            args = [ "%file" ];
          };
          auto-format = true;
        }
        {
          name = "json";
          language-servers = [ "vscode-json" ];
          formatter = {
            command = "prettierd";
            args = [ "%file" ];
          };
          auto-format = true;
        }
        {
          name = "html";
          language-servers = [ "vscode-json" ];
          formatter = {
            command = "prettierd";
            args = [ "%file" ];
          };
          auto-format = true;
        }
        {
          name = "css";
          language-servers = [ "vscode-json" ];
          formatter = {
            command = "prettierd";
            args = [ "%file" ];
          };
          auto-format = true;
        }

        # Rust
        {
          name = "rust";
          language-servers = [ "rust-analyzer" ];
          formatter = { command = "rustfmt"; };
          auto-format = true;
        }

        # Go
        {
          name = "go";
          language-servers = [ "gopls" ];
          formatter = { command = "gofmt"; };
          auto-format = true;
        }

        # Nix
        {
          name = "nix";
          language-servers = [ "nixd" ];
          formatter = { command = "nixfmt"; };
          auto-format = true;
        }

        # Lua
        {
          name = "lua";
          language-servers = [ "lua" ];
          formatter = {
            command = "stylua";
            args = [ "-" ];
          };
          auto-format = true;
        }

        # Bash/Sh
        {
          name = "bash";
          language-servers = [ "bash" ];
          formatter = {
            command = "shfmt";
            args = [ "-i" "2" "-bn" "-ci" ];
          };
          auto-format = true;
        }

        # YAML / Markdown / SQL
        {
          name = "yaml";
          language-servers = [ "yaml" ];
          auto-format = false;
        }
        {
          name = "markdown";
          language-servers = [ "marksman" ];
          auto-format = false;
        }
        {
          name = "sql";
          language-servers = [ "sqls" ];
          formatter = {
            command = "pg_format";
            args = [ "-" ];
          };
          auto-format = false;
        }
      ];
    };

    themes.catppuccin_mocha_transparent = ''
      inherits = "catppuccin_mocha"
      "ui.background" = {}
      # При желании можно «расстеклить» и другие элементы:
      "ui.statusline" = {}
      "ui.popup"      = {}
      "ui.menu"       = {}
      "ui.window"     = {}
    '';
  };
}

