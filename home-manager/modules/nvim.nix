{ pkgs, ... }: {
  home.packages = with pkgs; [
    lua-language-server
    nodePackages.vscode-langservers-extracted
    nodePackages.bash-language-server
    nodePackages.yaml-language-server
    nodePackages.dockerfile-language-server-nodejs
    nodePackages.typescript-language-server
    nodePackages.prettier
    cbfmt
    yq
    yamlfmt
    ruff
    typstyle
    taplo
    luajit
    pyright
    nixfmt-classic
    nil
    stylua
    shfmt
    shellcheck
    yamllint
    hadolint
    go
    basedpyright
    protobuf_29
  ];

  programs.neovim = {
    enable = true;
    defaultEditor = true;
    viAlias = true;
    vimAlias = true;

    extraPackages = with pkgs; [ tree-sitter ];

    extraLuaConfig = # lua
      ''
        vim.opt.packpath = vim.fn.stdpath('data') .. '/site'
        vim.opt.runtimepath = {
          vim.fn.stdpath('config'),
          vim.env.VIMRUNTIME,
          vim.fn.stdpath('data') .. '/site',
        }

        local data_dir = vim.fn.stdpath('data')
        local site_dir = data_dir .. '/site'
        local function ensure_dir(dir)
          if vim.fn.isdirectory(dir) == 0 then
            vim.fn.mkdir(dir, 'p')
          end
        end

        ensure_dir(site_dir)
        ensure_dir(site_dir .. '/pack')
        ensure_dir(site_dir .. '/after')
      '';
  };

  home.file.".config/nvim/" = {
    source = ../configs/nvim;
    recursive = true;
  };

  home.file.".local/share/nvim/site/pack/.keep".text = "";
  home.file.".local/share/nvim/site/start/.keep".text = "";
  home.file.".local/share/nvim/site/after/pack/.keep".text = "";
  home.file.".local/share/nvim/site/after/start/.keep".text = "";
}
