local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local switch = require("gator").module("workspace").switch

local source = helpers.tempdir("workspace-switch-source")
local target = helpers.tempdir("workspace-switch-target")
local resolved_source = assert(vim.uv.fs_realpath(source))
helpers.write(source .. "/lib/rebound.txt", "source\n")
helpers.write(target .. "/lib/rebound.txt", "target\n")
helpers.write(source .. "/lib/modified.txt", "source\n")
helpers.write(target .. "/lib/modified.txt", "target\n")

vim.cmd("tabnew")
vim.cmd("edit " .. vim.fn.fnameescape(source .. "/lib/rebound.txt"))
local rebound_window = vim.api.nvim_get_current_win()
local source_buffer = vim.api.nvim_get_current_buf()
vim.cmd("vsplit")
vim.cmd("edit " .. vim.fn.fnameescape(source .. "/lib/modified.txt"))
local modified_window = vim.api.nvim_get_current_win()
vim.api.nvim_buf_set_lines(0, 0, 1, false, { "changed" })
assert(vim.bo.modified, "fixture must create a modified buffer")

local value = switch.open({ from = source, to = target })
local resolved_target = assert(vim.uv.fs_realpath(target))
assert(
	vim.api.nvim_buf_get_name(vim.api.nvim_win_get_buf(rebound_window)) == resolved_target .. "/lib/rebound.txt",
	"existing files must rebind"
)
assert(
	vim.api.nvim_buf_get_name(vim.api.nvim_win_get_buf(modified_window)) == resolved_source .. "/lib/modified.txt",
	"modified user buffers must remain open"
)
assert(vim.api.nvim_buf_is_valid(source_buffer), "source buffers must remain available after rebinding")
assert(value.skipped[1].reason == "modified", "skipped modified buffers must be explicit")
assert(vim.fn.getcwd() == resolved_target, "switching must use the tab-local workspace cwd")
vim.cmd("tabclose!")
