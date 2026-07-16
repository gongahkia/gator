local adapters = require("gator").module("adapters")
local function contract(model, usage)
	local supported = { available = true, modes = { "native" } }
	return adapters.capabilities.new({
		provider = "codex",
		transport = supported,
		auth = supported,
		session = supported,
		permission = supported,
		model = model,
		command = supported,
		tool = supported,
		context = supported,
		usage = usage,
	})
end
local value = adapters.model_usage.read({
	capabilities = contract({ available = true, modes = { "current" } }, { available = true, modes = { "current" } }),
	now = 100,
	max_age_ms = 10,
	probe = function()
		return {
			model = { name = "model-one", observed_at = 95 },
			usage = { input_tokens = 2, output_tokens = 3, observed_at = 100 },
		}
	end,
})
assert(value.model.available and value.model.name == "model-one", "fresh model data must be exposed")
assert(value.usage.available and value.usage.output_tokens == 3, "fresh usage data must be exposed")
local stale = adapters.model_usage.read({
	capabilities = contract(
		{ available = true, modes = { "current" } },
		{ available = false, reason = "usage unavailable" }
	),
	now = 100,
	max_age_ms = 1,
	probe = function()
		return { model = { name = "old", observed_at = 1 } }
	end,
})
assert(not stale.model.available and stale.model.reason == "adapter data is stale", "stale data must not be exposed")
assert(
	not stale.usage.available and stale.usage.reason == "usage unavailable",
	"unsupported usage must remain explicit"
)
