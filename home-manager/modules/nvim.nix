{ pkgs, ... }:
let
  treesitterWithGrammars = (pkgs.vimPlugins.nvim-treesitter.withPlugins (p: [
    p.bash
    p.comment
    p.css
    p.dockerfile
    p.fish
    p.gitattributes
    p.gitignore
    p.go
    p.gomod
    p.gowork
    p.hcl
    p.javascript
    p.jq
    p.json5
    p.json
    p.lua
    p.make
    p.markdown
    p.nix
    p.python
    p.rust
    p.toml
    p.typescript
    p.vue
    p.yaml
  ]));
  pylsp = (pkgs.python312.withPackages
    (ps: with ps; [ python-lsp-server pylsp-mypy ]));
in {
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
    pylsp
    typstyle
    taplo
    ruff-lsp
    luajit
    pyright
    nixfmt-classic
    nil
    stylua
    shfmt
    ruff
    shellcheck
    yamllint
    hadolint
    hurl
    go
  ];

  programs.neovim = {
    enable = true;
    defaultEditor = true;
    viAlias = true;
    vimAlias = true;
    withNodeJs = true;
    withPython3 = true;

    extraPackages = with pkgs; [ xdg-utils tree-sitter ];

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

    plugins = with pkgs.vimPlugins; [
      treesitterWithGrammars
      luasnip
      friendly-snippets
    ];
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
