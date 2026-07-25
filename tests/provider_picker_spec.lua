local capabilities = require("gator").module("adapters").capabilities
local picker = require("gator.ui").provider_picker
local function contract(provider, transport, auth)
	local ready = { available = true, modes = { "native" } }
	return capabilities.new({
		provider = provider,
		transport = transport or ready,
		auth = auth or ready,
		session = ready,
		permission = ready,
		model = ready,
		command = ready,
		tool = ready,
		context = ready,
		usage = ready,
	})
end
local selected
picker.open({
	providers = {
		contract("codex"),
		contract("claude", { available = false, reason = "transport unavailable" }),
	},
	on_launch = function(value)
		selected = value
	end,
})
assert(require("gator.ui").picker.select(1).id == "codex", "provider picker must omit unavailable transports")
require("gator.ui").picker.confirm()
assert(
	selected.provider == "codex" and selected.capabilities.transport.available,
	"provider picker must pass only capability-ready providers to launch"
)
picker.open({
	providers = {
		contract("pi", nil, { available = true, modes = { "user_confirmed" } }),
	},
	on_launch = function(value)
		selected = value
	end,
})
assert(
	require("gator.ui").picker.select(1).label == "pi · user-confirmed; ready to launch",
	"provider picker must label user-confirmed providers distinctly"
)
require("gator.ui").picker.confirm()
assert(selected.provider == "pi", "provider picker must permit explicitly user-confirmed providers")
