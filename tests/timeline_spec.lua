local timeline = require("gator.ui").timeline
local calls = {
	{
		id = "call-one",
		provider = "codex",
		session_id = "native-one",
		name = "read_file",
		arguments = "path=README.md token=fixture-secret",
		approval = "granted",
		output = "# Gator\ntoken=fixture-secret",
		status = "succeeded",
	},
	{
		id = "call-two",
		provider = "claude",
		session_id = "native-two",
		name = "apply_patch",
		arguments = '{"path":"lua/gator/init.lua"}',
		approval = "denied",
		failure = "user declined\ntoken=fixture-secret",
		status = "failed",
	},
}

local window = timeline.open({ calls = calls })
assert(vim.api.nvim_win_is_valid(window), "timeline opening must create a window")
local content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(content:find("arguments: path=README.md", 1, true), "timeline must render provider arguments")
assert(content:find("failure: user declined", 1, true), "timeline must render failures")
assert(not content:find("fixture-secret", 1, true), "timeline must redact rendered tool details")
assert(timeline.toggle("call-one"), "timeline calls must be collapsible")
content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(not content:find("README.md", 1, true), "collapsed calls must hide arguments and output")
timeline.update(calls)
assert(not timeline.toggle("call-one"), "timeline updates must preserve collapse state")
assert(timeline.close(), "timeline close must report success")

local large = vim.tbl_extend("force", calls[1], {
	id = "call-large",
	output = string.rep(string.rep("x", 256) .. "\n", 1000),
	status = "running",
})
window = timeline.open({ calls = { large } })
local lines = vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false)
assert(
	#lines <= 205 and table.concat(lines, "\n"):find("output truncated in Gator", 1, true),
	"timeline must bound large retained output and disclose truncation"
)
assert(
	timeline.inspect().output_bytes <= timeline.inspect().max_output_bytes,
	"timeline must not retain unbounded streamed output"
)
assert(timeline.inspect().motion_active, "focused running timelines must own one active status timer")
timeline.update({ large })
timeline.update({ large })
assert(timeline.inspect().refresh_pending, "large stream updates must coalesce pending layout renders")
assert(
	vim.wait(100, function()
		return not timeline.inspect().refresh_pending
	end),
	"coalesced stream updates must eventually render"
)
vim.cmd("new")
assert(
	vim.wait(100, function()
		return not timeline.inspect().motion_active
	end),
	"timeline inspection must pause when its view is inactive"
)
vim.api.nvim_set_current_win(window)
assert(
	vim.wait(100, function()
		return timeline.inspect().motion_active
	end),
	"focusing a running timeline must resume its status timer"
)
vim.cmd("wincmd p")
assert(
	vim.wait(100, function()
		return not timeline.inspect().motion_active
	end),
	"inactive timelines must pause their status timer"
)
vim.api.nvim_set_current_win(window)
assert(timeline.close() and not timeline.inspect(), "closing timelines must cancel all status timers")

local ok = pcall(timeline.open, { calls = { vim.tbl_extend("force", calls[1], { status = "failed", failure = nil }) } })
assert(not ok, "failed calls without failures must fail explicitly")
ok = pcall(timeline.open, { calls = { vim.tbl_extend("force", calls[1], { output = "" }) } })
assert(not ok, "empty tool outputs must fail explicitly")
