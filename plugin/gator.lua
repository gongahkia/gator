if vim.g.loaded_gator == 1 then return end
vim.g.loaded_gator = 1

vim.api.nvim_create_user_command("Gator", function() require("gator").open() end, {})
vim.api.nvim_create_user_command("GatorHealth", function() require("gator").health() end, {})
