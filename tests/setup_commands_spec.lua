local command =
	"lua local g=require('gator').setup(); assert(vim.fn.exists(':Gator') == 2); assert(vim.fn.exists(':GatorHealth') == 2); assert(vim.fn.exists(':GatorRuns') == 2); assert(vim.fn.exists(':GatorCompletion') == 2); assert(vim.fn.exists(':GatorBetaReadiness') == 0); g.setup()"
local result = vim.system({
	vim.v.progpath,
	"--headless",
	"--noplugin",
	"-u",
	"NONE",
	"-i",
	"NONE",
	"--cmd",
	"set rtp+=" .. vim.g.gator_test.root,
	"--cmd",
	command,
	"--cmd",
	"qa!",
}, { cwd = vim.g.gator_test.root, text = true }):wait()
assert(result.code == 0, "direct setup must register Gator commands without loading plugin/gator.lua")
