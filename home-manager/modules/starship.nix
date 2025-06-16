{ config, ... }: {
  home.sessionVariables = {
    STARSHIP_CACHE = "${config.xdg.cacheHome}/starship";
  };

  programs.starship = {
    enable = true;
    enableZshIntegration = true;

    settings = {
      # ============================================================
      # CATPPUCCIN THEME
      # ============================================================

      # palette = "catppuccin_latte";
      # palette = "catppuccin_frappe";
      # palette = "catppuccin_macchiato";
      palette = "catppuccin_mocha";

      format =
        "$os$username\${custom.is_folder_config}$directory$git_branch$c$rust$golang$nodejs$php$java$kotlin$haskell$lua$python$cmake$docker_context$meson$cmd_duration$line_break$character";

      add_newline = true;

      os = {
        disabled = false;
        style = "bg:color_mantle fg:color_text";
        symbols = {
          Windows = "󰍲 ";
          Ubuntu = "󰕈 ";
          SUSE = " ";
          Raspbian = "󰐿 ";
          Mint = "󰣭 ";
          Macos = "󰀵 ";
          Manjaro = " ";
          Linux = "󰌽 ";
          Gentoo = "󰣨 ";
          Fedora = "󰣛 ";
          Alpine = " ";
          Amazon = " ";
          Android = " ";
          Arch = "󰣇 ";
          Artix = "󰣇 ";
          CentOS = " ";
          Debian = "󰣚 ";
          Redhat = "󱄛 ";
          RedHatEnterprise = "󱄛 ";
        };
      };

      cmd_duration = {
        min_time = 1000;
        show_notifications = false;
        min_time_to_notify = 10000;
        show_milliseconds = true;
        format = "[$duration ]($style)";
        style = "fg:color_pink bg:color_mantle";
      };

      directory = {
        truncate_to_repo = true;
        format = "[([$read_only ]($style fg:color_red))$path ]($style)";
        style = "fg:color_text bg:color_mantle";
        truncation_length = 5;
        read_only = "";
      };

      git_branch = {
        symbol = "";
        format = "[$symbol $branch(:$remote_branch) ]($style)";
        style = "fg:color_red bg:color_mantle";
      };

      git_status = { disabled = false; };

      git_commit = {
        disabled = true;
        only_detached = false;
        tag_disabled = false;
        format = "[$tag #$hash ]($style)";
        style = "fg:color_red bg:color_mantle";
        commit_hash_length = 4;
        tag_symbol = "";
      };

      c = {
        symbol = "gcc: ";
        format = "[$symbol($version )]($style)";
        style = "fg:color_lavender bg:color_mantle";
      };

      docker_context = {
        symbol = " ";
        format = "[$symbol$context ]($style)";
        style = "fg:color_blue bg:color_mantle";
      };

      golang = {
        symbol = "";
        format = "[$symbol ($version )]($style)";
        style = "fg:color_lavender bg:color_mantle";
      };

      lua = {
        symbol = "";
        format = "[$symbol ($version )]($style)";
        style = "fg:color_lavender bg:color_mantle";
      };

      nodejs = {
        symbol = "";
        format = "[$symbol ($version )]($style)";
        style = "fg:color_lavender bg:color_mantle";
      };

      python = {
        symbol = " ";
        format = "[\${symbol}\${version}]($style) ";
        style = "fg:color_lavender";
      };

      rust = {
        symbol = "";
        format = "[$symbol ($version )]($style)";
        style = "fg:color_lavender bg:color_mantle";
      };

      cmake = {
        format = "[$symbol($version )]($style)";
        style = "fg:color_lavender bg:color_mantle";
      };

      meson = {
        symbol = "Meson: ";
        format = "[$symbol(\${project}) ]($style)";
        style = "fg:color_lavender bg:color_mantle";
      };

      custom = {
        is_folder_config = {
          disabled = true;
          when = "echo $PWD | grep -q '/dotfiles'";
          symbol = " ";
          format = "[$symbol]($style)";
          style = "fg:color_blue bg:color_mantle";
        };
      };

      env_var = {
        shell = {
          disabled = false;
          symbol = "";
          variable = "SHELL";
          format = "[($symbol $env_value )]($style)";
          style = "fg:color_lavender bg:color_mantle";
        };
      };

      shell = { disabled = true; };

      line_break = { disabled = false; };

      status = {
        disabled = false;
        style = "fg:color_text bg:color_base";
        format = ''
          [
          (Exit code: [$maybe_int]($style fg:color_red))
          (Exit status: [$common_meaning]($style fg:color_peach))
          (Signal: [$signal_name]($style fg:color_yellow))]
          ($style)
        '';
      };

      character = {
        format = "> ";
        disabled = false;
      };

      palettes = {
        catppuccin_latte = {
          color_rosewater = "#dc8a78";
          color_flamingo = "#dd7878";
          color_pink = "#ea76cb";
          color_mauve = "#8839ef";
          color_red = "#d20f39";
          color_maroon = "#e64553";
          color_peach = "#fe640b";
          color_yellow = "#df8e1d";
          color_green = "#40a02b";
          color_teal = "#179299";
          color_sky = "#04a5e5";
          color_sapphire = "#209fb5";
          color_blue = "#1e66f5";
          color_lavender = "#7287fd";
          color_text = "#4c4f69";
          color_subtext1 = "#5c5f77";
          color_subtext0 = "#6c6f85";
          color_overlay2 = "#7c7f93";
          color_overlay1 = "#8c8fa1";
          color_overlay0 = "#9ca0b0";
          color_surface2 = "#acb0be";
          color_surface1 = "#bcc0cc";
          color_surface0 = "#ccd0da";
          color_base = "#eff1f5";
          color_mantle = "#e6e9ef";
          color_crust = "#dce0e8";
        };

        catppuccin_frappe = {
          color_rosewater = "#f2d5cf";
          color_flamingo = "#eebebe";
          color_pink = "#f4b8e4";
          color_mauve = "#ca9ee6";
          color_red = "#e78284";
          color_maroon = "#ea999c";
          color_peach = "#ef9f76";
          color_yellow = "#e5c890";
          color_green = "#a6d189";
          color_teal = "#81c8be";
          color_sky = "#99d1db";
          color_sapphire = "#85c1dc";
          color_blue = "#8caaee";
          color_lavender = "#babbf1";
          color_text = "#c6d0f5";
          color_subtext1 = "#b5bfe2";
          color_subtext0 = "#a5adce";
          color_overlay2 = "#949cbb";
          color_overlay1 = "#838ba7";
          color_overlay0 = "#737994";
          color_surface2 = "#626880";
          color_surface1 = "#51576d";
          color_surface0 = "#414559";
          color_base = "#303446";
          color_mantle = "#292c3c";
          color_crust = "#232634";
        };

        catppuccin_macchiato = {
          color_rosewater = "#f4dbd6";
          color_flamingo = "#f0c6c6";
          color_pink = "#f5bde6";
          color_mauve = "#c6a0f6";
          color_red = "#ed8796";
          color_maroon = "#ee99a0";
          color_peach = "#f5a97f";
          color_yellow = "#eed49f";
          color_green = "#a6da95";
          color_teal = "#8bd5ca";
          color_sky = "#91d7e3";
          color_sapphire = "#7dc4e4";
          color_blue = "#8aadf4";
          color_lavender = "#b7bdf8";
          color_text = "#cad3f5";
          color_subtext1 = "#b8c0e0";
          color_subtext0 = "#a5adcb";
          color_overlay2 = "#939ab7";
          color_overlay1 = "#8087a2";
          color_overlay0 = "#6e738d";
          color_surface2 = "#5b6078";
          color_surface1 = "#494d64";
          color_surface0 = "#363a4f";
          color_base = "#24273a";
          color_mantle = "#1e2030";
          color_crust = "#181926";
        };

        catppuccin_mocha = {
          color_rosewater = "#f5e0dc";
          color_flamingo = "#f2cdcd";
          color_pink = "#f5c2e7";
          color_mauve = "#cba6f7";
          color_red = "#f38ba8";
          color_maroon = "#eba0ac";
          color_peach = "#fab387";
          color_yellow = "#f9e2af";
          color_green = "#a6e3a1";
          color_teal = "#94e2d5";
          color_sky = "#89dceb";
          color_sapphire = "#74c7ec";
          color_blue = "#89b4fa";
          color_lavender = "#b4befe";
          color_text = "#cdd6f4";
          color_subtext1 = "#bac2de";
          color_subtext0 = "#a6adc8";
          color_overlay2 = "#9399b2";
          color_overlay1 = "#7f849c";
          color_overlay0 = "#6c7086";
          color_surface2 = "#585b70";
          color_surface1 = "#45475a";
          color_surface0 = "#313244";
          color_base = "#1e1e2e";
          # color_mantle = "#181825";
          color_mantle = "#0000000";
          color_crust = "#11111b";
        };
      };
    };
  };

  # ============================================================
  # PURE THEME (альтернативная конфигурация)
  # ============================================================

  # programs.starship.settings = {
  #   # Pure-inspired минималистичная тема
  #   format = "$directory$git_branch$git_status$line_break$character";
  #   right_format = "$cmd_duration";
  #   add_newline = false;
  #   
  #   character = {
  #     format = "$symbol ";
  #     success_symbol = "[❯](purple)";
  #     error_symbol = "[❯](red)";
  #     vicmd_symbol = "[❮](green)";
  #   };
  #   
  #   directory = {
  #     format = "[$path]($style) ";
  #     style = "cyan";
  #     truncation_length = 0;
  #     truncate_to_repo = false;
  #   };
  #   
  #   git_branch = {
  #     format = "[$branch]($style) ";
  #     style = "bright-black";
  #   };
  #   
  #   git_status = {
  #     format = "[$all_status$ahead_behind]($style) ";
  #     style = "red";
  #   };
  #   
  #   cmd_duration = {
  #     format = "[$duration]($style)";
  #     style = "yellow";
  #     min_time = 2000;
  #   };
  #   
  #   line_break = {
  #     disabled = false;
  #   };
  # };

  # ============================================================
  # MINIMAL THEME (еще одна альтернатива - ультра-минимализм)
  # ============================================================

  # programs.starship.settings = {
  #   # Ультра-минималистичная конфигурация
  #   format = "$directory$git_branch$character";
  #   add_newline = false;
  #   
  #   character = {
  #     format = "$symbol ";
  #     success_symbol = "[>](bold green)";
  #     error_symbol = "[>](bold red)";
  #   };
  #   
  #   directory = {
  #     format = "[$path]($style) ";
  #     style = "bold blue";
  #     truncation_length = 2;
  #   };
  #   
  #   git_branch = {
  #     format = "[$branch]($style) ";
  #     style = "purple";
  #   };
  # };
}
