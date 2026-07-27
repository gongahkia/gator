local retention = require("gator").module("core").retention
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local root = helpers.tempdir("retention")
local paths = {}
local ages = {}
for _, category in ipairs(retention.categories) do
	paths[category] = root .. "/" .. category
	ages[category] = 10
end
helpers.write(paths.transcripts .. "/stale.log", "stale")
helpers.write(paths.transcripts .. "/fresh.log", "fresh")
helpers.write(paths.indices .. "/stale.idx", "stale")
vim.uv.fs_utime(paths.transcripts .. "/stale.log", 1, 1)
vim.uv.fs_utime(paths.transcripts .. "/fresh.log", 95, 95)
vim.uv.fs_utime(paths.indices .. "/stale.idx", 1, 1)

local manager = retention.new({ root = root, paths = paths, max_age = ages })
local plan = manager:plan(100)
assert(#plan == 2, "retention plans must include only stale managed files")
assert(plan[1].path:find("stale", 1, true), "retention plans must be deterministic")
local ok = pcall(manager.prune, manager, plan, false)
assert(not ok, "retention cleanup must require explicit confirmation")
assert(#manager:prune(plan, true) == 2, "confirmed retention cleanup must remove planned files")
assert(vim.fn.filereadable(paths.transcripts .. "/fresh.log") == 1, "retention cleanup must preserve fresh files")

helpers.write(paths.telemetry .. "/scheduled.log", "stale")
vim.uv.fs_utime(paths.telemetry .. "/scheduled.log", 1, 1)
local timer, planned = {}, nil
timer.start = function(self, delay, repeat_ms, callback)
	self.delay, self.repeat_ms, self.callback = delay, repeat_ms, callback
end
timer.stop = function(self)
	self.stopped = true
end
timer.close = function(self)
	self.closed = true
end
local schedule = manager:schedule({
	interval_ms = 5,
	timer = timer,
	now = function()
		return 100
	end,
	on_plan = function(value)
		planned = value
	end,
	confirm = true,
})
timer.callback()
assert(
	vim.wait(100, function()
		return planned ~= nil
	end)
		and #planned == 1
		and vim.fn.filereadable(paths.telemetry .. "/scheduled.log") == 0,
	"scheduled retention must plan and prune only after explicit scheduler confirmation"
)
assert(
	schedule:cancel() and not schedule:cancel() and timer.stopped and timer.closed,
	"retention schedules must cancel cleanly"
)

local project = helpers.tempdir("project-retention") .. "/.gator"
helpers.write(project .. "/runs/run-old.json", '{"schema_version":1}')
helpers.write(project .. "/transcripts/run-active.md", "active")
vim.uv.fs_utime(project .. "/runs/run-old.json", 1, 1)
vim.uv.fs_utime(project .. "/transcripts/run-active.md", 1, 1)
local project_manager = retention.project(project, 1)
local project_plan = project_manager:plan(2 * 24 * 60 * 60, {
	exclude = function(path, category)
		return category == "transcripts" and path:find("run%-active", 1, false) ~= nil
	end,
})
assert(
	#project_plan == 1 and project_plan[1].category == "runs",
	"project retention must delete only named managed categories while preserving active exclusions"
)
