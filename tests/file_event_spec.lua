local file = require("gator").module("core").file_event

local change = file.change({
	id = "event-file-change",
	run_id = "run-file",
	provider = { name = "codex", session_id = "native-file" },
	sequence = 0,
	at = 1,
	path = "lua/gator/core.lua",
	kind = "modified",
	before = "token: private-value",
	after = "updated",
})
local diff = file.diff({
	id = "event-file-diff",
	run_id = "run-file",
	provider = { name = "codex" },
	sequence = 1,
	at = 2,
	patch = "token: private-value",
	paths = { "lua/gator/core.lua" },
})
assert(
	change.type == "file.change"
		and change.payload.before:find("private%-value") == nil
		and diff.type == "file.diff"
		and diff.payload.patch:find("private%-value") == nil,
	"file event normalization must redact diffs while retaining relative paths"
)
assert(not pcall(file.change, {
	id = "event-file-invalid",
	run_id = "run-file",
	provider = { name = "codex" },
	sequence = 2,
	at = 3,
	path = "../secret",
	kind = "modified",
}), "file event normalization must reject unsafe paths")
