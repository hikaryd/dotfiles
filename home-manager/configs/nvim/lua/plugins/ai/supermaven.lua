return {
    {
        'supermaven-inc/supermaven-nvim',
        enabled = true,
        event = 'VeryLazy',
        config = function()
            require('supermaven-nvim').setup {}
        end,
    },
}
