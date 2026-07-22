local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local event = require("gator").module("core").run.event
local snapshot = require("gator").module("workspace").snapshot

local root = helpers.tempdir("workspace-snapshot")
local status = " M lua/gator/init.lua\0?? dist/bundle.js\0"
local function run(argv)
	if argv[2] == "rev-parse" and argv[3] == "--show-toplevel" then
		return { code = 0, stdout = root .. "\n" }
	end
	if argv[2] == "rev-parse" and argv[3] == "--verify" then
		return { code = 0, stdout = "0123456789abcdef\n" }
	end
	if argv[2] == "status" then
		return { code = 0, stdout = status }
	end
	error("unexpected Git command")
end
local value = snapshot.capture({
	cwd = root,
	run_id = "run-write",
	write = true,
	at = 1,
	run = run,
})
assert(event(value).type == "workspace.git_snapshot", "snapshots must be valid persistent run evidence")
assert(
	value.payload.base_revision == "0123456789abcdef"
		and value.payload.working_tree.dirty
		and #value.payload.working_tree.changes == 2,
	"snapshots must record base revision and working-tree state"
)
status = " M lua/gator/context/handoff.lua\0"
local changed = snapshot.capture({ cwd = root, run_id = "run-write", write = true, at = 2, run = run })
assert(
	changed.id ~= value.id
		and changed.payload.working_tree.digest ~= value.payload.working_tree.digest
		and changed.payload.working_tree.changes[1].path == "lua/gator/context/handoff.lua"
		and #value.payload.working_tree.changes == 2,
	"changed source files must produce a distinct snapshot without mutating prior evidence"
)
assert(
	not pcall(snapshot.capture, { cwd = root, run_id = "run-read", write = false }),
	"read-only runs must not silently skip evidence"
)
