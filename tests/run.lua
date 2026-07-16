local root = vim.g.gator_test.root
local helpers = dofile(root .. "/tests/helpers.lua")
local pattern = vim.env.GATOR_TEST_GLOB or "*_spec.lua"
local specs = vim.fn.glob(root .. "/tests/" .. pattern, false, true)

table.sort(specs)

local ok, err = xpcall(function()
	assert(#specs > 0, "Gator test runner found no specs for " .. pattern)
	for _, spec in ipairs(specs) do
		dofile(spec)
	end
	local gator = require("gator")
	if not gator._state then
		gator.setup()
	end
	gator._test()
end, debug.traceback)

helpers.cleanup()

if not ok then
	vim.api.nvim_err_writeln(err)
	vim.cmd("cquit 1")
end

vim.cmd("qa!")
