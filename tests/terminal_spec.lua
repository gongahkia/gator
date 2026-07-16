local terminal = require("gator").module("adapters").terminal
local launched, stopped = {}, {}
local manager = terminal.new({
	termopen = function(command, opts)
		launched = { command = command, opts = opts }
		return 42
	end,
	jobstop = function(job_id)
		table.insert(stopped, job_id)
	end,
})
assert(manager:inspect().available, "terminal capability must report availability")
local session = manager:open({ id = "terminal-one", command = { "agent", "run" }, cwd = "/tmp" })
assert(
	launched.command[1] == "agent" and session.job_id == 42,
	"terminal fallback must launch an attached visible session"
)
assert(manager:attach("terminal-one") == session.window, "terminal sessions must be attachable")
assert(manager:close("terminal-one") and stopped[1] == 42, "terminal close must stop and clean up its job")
local unavailable = terminal.new({ termopen = false })
assert(not unavailable:inspect().available, "unavailable terminal capability must be explicit")
local ok = pcall(unavailable.open, unavailable, { id = "missing", command = { "agent" } })
assert(not ok, "unavailable terminal fallback must fail explicitly")
