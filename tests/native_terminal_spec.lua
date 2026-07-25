local terminal = require("gator.adapters.terminal")

local options, stopped, exited = nil, nil, nil
local manager = terminal.new({
	termopen = function(command, value)
		options = { command = command, value = value }
		return 7
	end,
	jobstop = function(id)
		stopped = id
		return 1
	end,
})

local opened = manager:open({
	id = "native-terminal",
	cwd = vim.g.gator_test.root,
	command = { "provider", "prompt" },
	on_exit = function(value)
		exited = value
	end,
})
assert(
	opened.job_id == 7 and options.command[1] == "provider" and options.value.cwd == vim.g.gator_test.root,
	"native terminals must retain argv and cwd"
)
options.value.on_exit(7, 0, "exit")
assert(
	exited.id == "native-terminal" and exited.code == 0 and not pcall(manager.attach, manager, "native-terminal"),
	"terminal exit must release attach state and report status"
)
assert(not manager:close("native-terminal") and stopped == nil, "exited terminals must not be stopped again")
