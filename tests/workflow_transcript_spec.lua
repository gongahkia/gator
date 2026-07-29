local config = require("gator.config")
local state = require("gator.state")
local workflow = require("gator.workflow")
local capture = require("gator.context.capture")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("workflow-transcript")
assert(vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0, "fixture must initialize Git")
helpers.write(root .. "/main.lua", "return true\n")
assert(
	vim.system({ "git", "add", "main.lua" }, { cwd = root, text = true }):wait().code == 0,
	"fixture must stage baseline"
)
assert(
	vim.system({ "git", "-c", "user.name=Gator", "-c", "user.email=gator@example.invalid", "commit", "-qm", "base" }, {
		cwd = root,
		text = true,
	})
		:wait().code == 0,
	"fixture must commit baseline"
)
vim.cmd("enew!")
local buffer = vim.api.nvim_get_current_buf()
vim.api.nvim_buf_set_name(buffer, root .. "/main.lua")
vim.api.nvim_buf_set_lines(buffer, 0, -1, false, { "return true" })
local value = workflow.new({
	state = state.new(config.resolve({ providers = { pi = { user_confirmed = true } } }), { supported = true }),
	root = root,
	readiness = function()
		return { { provider = "pi", available = true, readiness_state = "user_confirmed" } }
	end,
	structured = {
		open = function()
			return {
				send = function()
					return true
				end,
				cancel = function()
					return true
				end,
			}
		end,
	},
	loading = {
		open = function()
			return { close = function() end }
		end,
	},
})
local run = value:launch({
	provider = "pi",
	transport = "chat",
	objective = "Inspect transcript streaming",
	capture = capture.current({ buffer = buffer, first_line = 1, last_line = 1 }),
})

assert(value:append_transcript(run.id, "assistant", "I"), "first assistant fragment must persist")
assert(value:append_transcript(run.id, "assistant", "'ll inspect", true), "assistant fragments must append")
assert(
	value:transcript(value:run(run.id)) == "## assistant\nI'll inspect",
	"streamed assistant fragments must persist as one message"
)
value.transcripts[run.id] = nil
assert(value:append_transcript(run.id, "assistant", " this", true), "rehydrated fragments must append")
assert(
	value:transcript(value:run(run.id)) == "## assistant\nI'll inspect this",
	"rehydrated transcript appends must preserve prior output"
)
assert(require("gator.ui.workspace").close(), "test workspace must close cleanly")
