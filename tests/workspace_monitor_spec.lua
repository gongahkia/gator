local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local monitor = require("gator").module("workspace").monitor

local root = helpers.tempdir("workspace-monitor")
local resolved_root = assert(vim.uv.fs_realpath(root))
local value = monitor.inspect({
	root = root,
	run = function(argv, cwd)
		assert(
			argv[1] == "git" and argv[2] == "status" and cwd == resolved_root,
			"monitor must use Git porcelain status"
		)
		return { code = 0, stdout = " M lua/gator/init.lua\0R  new.lua\0old.lua\0?? untracked file\0" }
	end,
	alive = function(pid)
		return pid == 10
	end,
	agents = {
		{ id = "agent-live", provider = "codex", session_id = "native-one", pid = 10, state = "running" },
		{ id = "agent-detached", provider = "claude", session_id = "native-two", pid = 20, state = "running" },
		{ id = "agent-finished", provider = "gemini", session_id = "native-three", pid = 30, state = "completed" },
	},
})
assert(value.dirty, "changed Git paths must mark a workspace dirty")
assert(
	table.concat(value.changed_paths, ",") == "lua/gator/init.lua,new.lua,old.lua,untracked file",
	"monitoring must retain modified, renamed, and untracked paths"
)
assert(value.agents[1].live and not value.agents[1].detached, "live agents must remain attached")
assert(not value.agents[2].live and value.agents[2].detached, "missing active processes must be detached")
assert(not value.agents[3].live and not value.agents[3].detached, "terminal processes must not be detached")
assert(not pcall(monitor.inspect, {
	root = root,
	run = function()
		return { code = 1, stdout = "" }
	end,
}), "Git failures must be explicit")
