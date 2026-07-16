local timeline = require("gator.ui").timeline
local calls = {
	{
		id = "call-one",
		provider = "codex",
		session_id = "native-one",
		name = "read_file",
		arguments = '{"path":"README.md"}',
		approval = "granted",
		output = "# Gator",
		status = "succeeded",
	},
	{
		id = "call-two",
		provider = "claude",
		session_id = "native-two",
		name = "apply_patch",
		arguments = '{"path":"lua/gator/init.lua"}',
		approval = "denied",
		failure = "user declined",
		status = "failed",
	},
}

local window = timeline.open({ calls = calls })
assert(vim.api.nvim_win_is_valid(window), "timeline opening must create a window")
local content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(content:find('arguments: {"path"', 1, true), "timeline must render provider arguments")
assert(content:find("failure: user declined", 1, true), "timeline must render failures")
assert(timeline.toggle("call-one"), "timeline calls must be collapsible")
content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(not content:find("README.md", 1, true), "collapsed calls must hide arguments and output")
timeline.update(calls)
assert(not timeline.toggle("call-one"), "timeline updates must preserve collapse state")
assert(timeline.close(), "timeline close must report success")

local ok = pcall(timeline.open, { calls = { vim.tbl_extend("force", calls[1], { status = "failed", failure = nil }) } })
assert(not ok, "failed calls without failures must fail explicitly")
ok = pcall(timeline.open, { calls = { vim.tbl_extend("force", calls[1], { output = "" }) } })
assert(not ok, "empty tool outputs must fail explicitly")
