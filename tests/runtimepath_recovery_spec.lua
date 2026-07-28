local root = vim.fs.normalize(vim.g.gator_test.root)
vim.opt.runtimepath:remove(root)
require("gator").setup()
vim.api.nvim_exec_autocmds("VimEnter", {})
local found = false
for _, entry in ipairs(vim.opt.runtimepath:get()) do
	if vim.fs.normalize(entry) == root then
		found = true
	end
end
assert(found, "setup must retain Gator on runtimepath for :checkhealth gator after user config loads")
assert(
	#vim.api.nvim_get_runtime_file("lua/**/gator/health.lua", true) > 0,
	"retained runtimepath must expose the Gator healthcheck module"
)
