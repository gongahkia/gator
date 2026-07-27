local config = require("gator.config")
local state = require("gator.state")
local workflow = require("gator.workflow")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("workflow-journal-preflight")
assert(vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0, "fixture must initialize Git")
helpers.write(root .. "/main.lua", "local secret = 'token=private-value'\nreturn secret\n")
assert(vim.system({ "git", "add", "main.lua" }, { cwd = root, text = true }):wait().code == 0, "fixture must stage baseline")
assert(vim.system({ "git", "-c", "user.name=Gator", "-c", "user.email=gator@example.invalid", "commit", "-qm", "base" }, {
	cwd = root,
	text = true,
}):wait().code == 0, "fixture must commit baseline")

vim.cmd("enew!")
local buffer = vim.api.nvim_get_current_buf()
vim.api.nvim_buf_set_name(buffer, root .. "/main.lua")
vim.api.nvim_buf_set_lines(buffer, 0, -1, false, { "local secret = 'token=private-value'", "return secret" })
local capture = require("gator.context.capture").current({ buffer = buffer, first_line = 1, last_line = 2 })
local opened, sent = nil, {}
local value = workflow.new({
	state = state.new(config.resolve({ providers = { pi = { user_confirmed = true } } }), { supported = true }),
	root = root,
	readiness = function()
		return { { provider = "pi", available = true, readiness_state = "user_confirmed", version = "0.82.0" } }
	end,
	structured = {
		open = function(_, opts)
			opened = opts
			opts.on_session({ id = "pi-journal", resume_supported = true })
			return {
				send = function(message)
					table.insert(sent, message)
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
	loading = { open = function() return { close = function() end } end },
})
local run = value:launch({ provider = "pi", transport = "chat", objective = "Check selection", capture = capture })
local events = value:events(run.id)
local types = {}
for _, event in ipairs(events) do
	types[event.type] = event.payload
end
vim.print(types)
assert(
	opened and types["run.created"] and types["provider.selected"].version == "0.82.0" and types["trust.applied"]
		and types["context.prepared"].artifacts[1].path == root .. "/main.lua"
		and types["context.prepared"].redactions == 1
		and types["context.sent"],
	"launches must journal provider version, trust, and passive context metadata before sending"
)
assert(
	not table.concat(vim.fn.readfile(root .. "/.gator/events/" .. run.id .. ".jsonl"), "\n"):find("private%-value"),
	"the context journal must not persist selected source text"
)
assert(value:send_context({ run_id = run.id, kind = "selection", buffer = buffer, first_line = 1, last_line = 1 }), "context send must succeed")
events = value:events(run.id)
local prepared = events[#events - 1]
assert(
	#sent == 1
		and prepared.type == "context.prepared"
		and prepared.payload.purpose == "send"
		and prepared.payload.artifacts[1].first_line == 1,
	"follow-up context must be passively logged with exact range metadata before it is sent"
)
assert(require("gator.ui.conversation").close(), "test chat panel must remain ephemeral")
