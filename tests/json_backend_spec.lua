local filesystem = require("gator").module("core").filesystem
local backend = require("gator").module("core").json_backend
local run = require("gator").module("core").run
local task = require("gator").module("core").task

local files = {}
local fs = filesystem.new({
	readable = function(path)
		return files[path] ~= nil
	end,
	read = function(path)
		return files[path]
	end,
	mkdir = function()
		return true
	end,
	write = function(path, value)
		files[path] = value
		return true
	end,
	rename = function(source, target)
		files[target], files[source] = files[source], nil
		return true
	end,
	remove = function(path)
		files[path] = nil
		return true
	end,
})
local store = backend.open("/fixture/state.json", { filesystem = fs })
local item = task.new({ id = "task-json", objective = "token: private-value", created_at = 1, updated_at = 1 })
store:put_task(item)
local value = run.new({
	id = "run-json",
	task_id = "task-json",
	provider = { name = "codex", session_id = "native-json" },
	process = { pid = 1, executable = "codex" },
	workspace = { kind = "project", root = "/fixture" },
	state = "running",
	timing = { started_at = 1 },
	usage = {},
})
store:append_run(value)
store:append_run_event("run-json", {
	id = "event-json",
	run_id = "run-json",
	type = "stream.delta",
	at = 2,
	payload = { text = "token: private-value" },
})
assert(
	store:get_task("task-json").objective:find("private%-value") == nil
		and store:get_run("run-json").events[1].payload.text:find("private%-value") == nil
		and vim.json.decode(store:export_bundle({ task_id = "task-json" })).runs[1].provider.session_id
			== "native-json",
	"JSON backends must persist validated redacted records while preserving native session identity"
)
local broken = backend.open("/fixture/broken.json", {
	filesystem = filesystem.new({
		mkdir = function()
			return true
		end,
		write = function()
			return false
		end,
	}),
})
assert(not pcall(broken.put_task, broken, item), "JSON backend write failures must remain explicit")
