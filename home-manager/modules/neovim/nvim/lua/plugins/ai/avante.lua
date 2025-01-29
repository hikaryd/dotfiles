return {
  'yetone/avante.nvim',
  event = 'VeryLazy',
  version = false,
  opts = {
    provider = 'mistral',
    auto_suggestions_provider = 'openrouterdeepseek',
    vendors = {
      openrouterdeepseek = {
        __inherited_from = 'openai',
        endpoint = 'https://openrouter.ai/api/v1',
        model = 'deepseek/deepseek-chat',
        api_key_name = 'OPENROUTER_API_KEY',
        temperature = 0.0,
        max_tokens = 8192,
      },
      mistral = {
        __inherited_from = 'openai',
        endpoint = 'https://openrouter.ai/api/v1',
        model = 'mistralai/codestral-2501',
        api_key_name = 'OPENROUTER_API_KEY',
        temperature = 0.0,
        max_tokens = 8192,
      },
      deepseek_r1 = {
        __inherited_from = 'openai',
        endpoint = 'https://openrouter.ai/api/v1',
        model = 'deepseek/deepseek-r1',
        api_key_name = 'OPENROUTER_API_KEY',
        temperature = 0.0,
        max_tokens = 50000,
      },
    },
    behaviour = {
      auto_suggestions = false,
      auto_set_highlight_group = true,
      auto_set_keymaps = true,
      auto_apply_diff_after_generation = true,
      support_paste_from_clipboard = true,
      minimize_diff = true,
    },
    hints = { enabled = true },
    windows = {
      position = 'right',
      wrap = true,
      width = 30,
      sidebar_header = {
        enabled = true,
        align = 'center',
        rounded = true,
      },
      input = {
        prefix = '> ',
        height = 8,
      },
      edit = {
        border = 'rounded',
        start_insert = true,
      },
      ask = {
        floating = false,
        start_insert = true,
        border = 'rounded',
        focus_on_apply = 'ours',
      },
    },
    highlights = {
      diff = {
        current = 'DiffText',
        incoming = 'DiffAdd',
      },
    },
    diff = {
      autojump = true,
      list_opener = 'copen',
      override_timeoutlen = 500,
    },
    file_selector = {
      provider = 'fzf',
      provider_opts = {},
    },
  },
  build = 'make',
  dependencies = {
    'stevearc/dressing.nvim',
    'nvim-lua/plenary.nvim',
    'MunifTanjim/nui.nvim',
    {
      'MeanderingProgrammer/render-markdown.nvim',
      lazy = false,
      opts = {
        file_types = { 'markdown', 'Avante' },
      },
      ft = { 'markdown', 'Avante' },
    },
    {
      'HakonHarnes/img-clip.nvim',
      event = 'VeryLazy',
      opts = {
        default = {
          embed_image_as_base64 = false,
          prompt_for_file_name = false,
          drag_and_drop = {
            insert_mode = true,
          },
        },
      },
    },
    {
      'ibhagwan/fzf-lua',
      dependencies = { 'nvim-tree/nvim-web-devicons' },
      opts = {},
    },
  },
}
