local accessibility = require("gator.ui.accessibility")
local capture = require("gator.context.capture")
local config = require("gator.config")
local conversation = require("gator.ui.workspace")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local state = require("gator.state")
local workflow = require("gator.workflow")

accessibility.configure({ keymaps = {}, screen_reader = false, icons = "none" })
local root = helpers.tempdir("workflow-stall")
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

local callbacks
local value = workflow.new({
	state = state.new(
		config.resolve({
			launch = { stall_after_ms = 25 },
			providers = { pi = { user_confirmed = true } },
		}),
		{ supported = true }
	),
	root = root,
	readiness = function()
		return { { provider = "pi", available = true, readiness_state = "user_confirmed" } }
	end,
	structured = {
		open = function(_, opts)
			callbacks = opts
			return {
				send = function()
					return true
				end,
				cancel = function()
					return true
				end,
				stop = function()
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
	objective = "Inspect the file",
	capture = capture.current({ buffer = buffer, first_line = 1, last_line = 1 }),
})
assert(
	vim.wait(500, function()
		return value:run(run.id).activity and value:run(run.id).activity.state == "stalled"
	end),
	"a silent structured startup must become visibly stalled after its configured threshold"
)
local stalled = table.concat(vim.api.nvim_buf_get_lines(0, 0, -1, false), "\n")
assert(stalled:find("Provider appears stalled", 1, true), "stalled chat UI must preserve cancel and detach choices")
callbacks.on_session({ id = "pi-stall-session", resume_supported = true })
assert(value:run(run.id).activity.state == "active", "a provider session must recover the stalled startup state")
assert(
	not table.concat(vim.api.nvim_buf_get_lines(0, 0, -1, false), "\n"):find("Provider appears stalled", 1, true),
	"a provider session must clear the stalled chat warning"
)
assert(
	vim.wait(500, function()
		return value:run(run.id).activity and value:run(run.id).activity.state == "stalled"
	end),
	"a silent structured turn must become stalled after session setup too"
)
callbacks.on_event("phase", "working")
assert(value:run(run.id).activity.state == "active", "a later provider event must recover the stalled state")
callbacks.on_event("settled")
assert(value:run(run.id).activity.state == "inactive", "settled turns must stop the stall watchdog")
local seen = {}
for _, event in ipairs(value:events(run.id)) do
	seen[event.type] = true
end
assert(
	seen["provider.stalled"] and seen["provider.recovered"],
	"stall transitions must be journalled without provider output"
)
assert(conversation.close(), "stall workspace must close cleanly")
