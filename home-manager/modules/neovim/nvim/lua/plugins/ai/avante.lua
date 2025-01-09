return {
  'yetone/avante.nvim',
  event = 'VeryLazy',
  lazy = false,
  version = false,
  opts = {
    provider = 'openrouterdeepseek',
    auto_suggestions_provider = 'openrouterdeepseek',
    claude = {
      endpoint = 'https://openrouter.ai/api/v1',
      model = 'anthropic/claude-3.5-sonnet:beta',
      temperature = 0,
      max_tokens = 8192,
      api_key_name = 'OPENROUTER_API_KEY',
    },
    vendors = {
      openrouterdeepseek = {
        __inherited_from = 'openai',
        endpoint = 'https://openrouter.ai/api/v1',
        model = 'deepseek/deepseek-chat',
        api_key_name = 'OPENROUTER_API_KEY',
        temperature = 0.0,
        max_tokens = 8192,
      },
      openrouterclaude = {
        __inherited_from = 'openai',
        endpoint = 'https://openrouter.ai/api/v1',
        model = 'anthropic/claude-3.5-sonnet:beta',
        api_key_name = 'OPENROUTER_API_KEY',
        temperature = 0.0,
        max_tokens = 8192,
      },
      openrouterllama = {
        __inherited_from = 'anthropic',
        endpoint = 'https://openrouter.ai/api/v1',
        model = 'meta-llama/llama-3.3-70b-instruct',
        api_key_name = 'OPENROUTER_API_KEY',
        temperature = 0.0,
        max_tokens = 8192,
      },
    },
    behaviour = {
      auto_suggestions = false, -- Experimental stage
      auto_set_highlight_group = true,
      auto_set_keymaps = true,
      auto_apply_diff_after_generation = true,
      support_paste_from_clipboard = false,
      minimize_diff = true, -- Whether to remove unchanged lines when applying a code block
    },
    hints = { enabled = true },
    windows = {
      position = 'right', -- the position of the sidebar
      wrap = true, -- similar to vim.o.wrap
      width = 30, -- default % based on available width
      sidebar_header = {
        enabled = true,
        align = 'center', -- left, center, right for title
        rounded = true,
      },
      input = {
        prefix = '> ',
        height = 8, -- Height of the input window in vertical layout
      },
      edit = {
        border = 'rounded',
        start_insert = true, -- Start insert mode when opening the edit window
      },
      ask = {
        floating = false, -- Open the 'AvanteAsk' prompt in a floating window
        start_insert = true, -- Start insert mode when opening the ask window
        border = 'rounded',
        focus_on_apply = 'ours', -- which diff to focus after applying
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
      --- @alias FileSelectorProvider "native" | "fzf" | "telescope" | string
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
