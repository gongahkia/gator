local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local sync = require("gator").module("github").sync

local root = helpers.tempdir("github-sync")
local first = root .. "/.gator/tasks/task-first.json"
local second = root .. "/.gator/tasks/task-second.json"
helpers.write(first, '{"version":1}')
helpers.write(second, '{"version":1}')
local refs = { ".gator/tasks/task-first.json", ".gator/tasks/task-second.json" }
local snapshots = sync.capture({
	cwd = root,
	refs = refs,
	run = function(argv)
		assert(argv[2] == "rev-parse", "sync capture must resolve the Git root")
		return { code = 0, stdout = root .. "\n" }
	end,
})
helpers.write(first, '{"version":2}')
local value = sync.inspect({
	cwd = root,
	snapshots = snapshots,
	run = function(argv)
		if argv[2] == "rev-parse" then
			return { code = 0, stdout = root .. "\n" }
		end
		if argv[2] == "status" then
			return { code = 0, stdout = " M .gator/tasks/task-first.json\nUU .gator/tasks/task-second.json\n" }
		end
		assert(argv[2] == "diff", "sync inspection must check merge conflicts")
		return { code = 0, stdout = ".gator/tasks/task-second.json\n" }
	end,
})
assert(
	#value.changes == 2
		and value.changes[1].ref == ".gator/tasks/task-first.json"
		and value.conflicts[1] == ".gator/tasks/task-second.json"
		and value.stale[1].ref == ".gator/tasks/task-first.json",
	"shared artifact sync must report Git changes, merge conflicts, and digest-stale local copies"
)
vim.fn.delete(second)
local missing = sync.inspect({
	cwd = root,
	snapshots = snapshots,
	run = function(argv)
		if argv[2] == "rev-parse" then
			return { code = 0, stdout = root .. "\n" }
		end
		return { code = 0, stdout = "" }
	end,
})
assert(
	#missing.stale == 2 and missing.stale[2].reason == "shared artifact is missing locally",
	"missing locally synchronized artifacts must be explicit stale records"
)
