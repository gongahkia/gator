local gator = require("gator")
local selection = require("gator.ui").selection

assert(gator._state, "selection actions require the initialized Gator state")
vim.api.nvim_buf_set_lines(0, 0, -1, false, { "first selected line", "second selected line", "other line" })
vim.bo.filetype = "gator-test"
vim.cmd("1,2GatorCaptureSelection task:task-selection")

local task_capture = gator._state.context.selections[1]
assert(
	task_capture.target.kind == "task" and task_capture.target.id == "task-selection",
	"task targets must remain explicit"
)
assert(task_capture.entry.kind == "selection", "selection actions must create selection context entries")
assert(task_capture.entry.provenance.source == "buffer", "selection actions must retain buffer provenance")
assert(
	task_capture.lines[1] == "first selected line" and #task_capture.lines == 2,
	"selection actions must retain selected lines"
)
assert(task_capture.language == "gator-test", "selection actions must retain buffer language")

vim.cmd("3GatorCaptureSelection session:codex:native-one")
local session_capture = gator._state.context.selections[2]
assert(
	session_capture.target.kind == "session"
		and session_capture.target.provider == "codex"
		and session_capture.target.id == "native-one",
	"session targets must retain opaque provider-native identity"
)
assert(session_capture.entry.transfer.eligible, "selection context must be transferable by explicit user action")

local ok = pcall(selection.capture, gator._state, "session::missing", { buffer = 0, first_line = 1, last_line = 1 })
assert(not ok, "invalid selection targets must fail explicitly")
