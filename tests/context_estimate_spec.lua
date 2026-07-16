local adapters = require("gator").module("adapters")
local estimate = require("gator").module("context").estimate
local supported = { available = true, modes = { "native" } }
local function contract(model)
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
		usage = supported,
	})
end
local counted = estimate.tokens({
	capabilities = contract({ available = true, modes = { "token_count" } }),
	text = "one two",
	count = function(text)
		assert(text == "one two", "provider counter must receive unchanged text")
		return 10
	end,
})
assert(counted.status == "estimated" and counted.tokens == 11, "token estimates must retain a conservative margin")
local unavailable = estimate.tokens({ capabilities = contract(supported), text = "one" })
assert(unavailable.status == "unavailable", "unadvertised provider counters must remain unavailable")
local failed = estimate.tokens({
	capabilities = contract({ available = true, modes = { "token_count" } }),
	text = "one",
	count = function()
		error("unavailable")
	end,
})
assert(failed.status == "unavailable", "failed provider counters must remain unavailable")
