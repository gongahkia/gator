local capabilities = require("gator").module("adapters").capabilities
local session = require("gator").module("core").session
local picker = require("gator.ui").session_picker
local ready = { available = true, modes = { "resume" } }
local function contract(provider, session_capability)
	local native = { available = true, modes = { "native" } }
	return capabilities.new({
		provider = provider,
		transport = native,
		auth = native,
		session = session_capability or ready,
		permission = native,
		model = native,
		command = native,
		tool = native,
		context = native,
		usage = native,
	})
end
local calls = {}
picker.open({
	sessions = {
		session.new({ task_id = "task-picker", provider = "codex", id = "native-one", owner = "provider" }),
		session.new({ task_id = "task-picker", provider = "claude", id = "native-two", owner = "provider" }),
	},
	capabilities = {
		codex = contract("codex"),
		claude = contract("claude", { available = false, reason = "resume unavailable" }),
	},
	resume = function(value)
		table.insert(calls, { "resume", value })
	end,
	attach = function(value)
		table.insert(calls, { "attach", value })
	end,
})
assert(require("gator.ui").picker.select(1).id == "codex:native-one", "session picker must omit non-resumable sessions")
require("gator.ui").picker.confirm()
assert(
	calls[1][1] == "resume" and calls[2][1] == "attach" and calls[2][2].owner == "provider",
	"session picker must resume then attach the provider-owned session"
)
