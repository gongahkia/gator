if vim.g.loaded_gator == 1 then
	return
end
vim.g.loaded_gator = 1

require("gator.commands").register()
