local capabilities = require("gator").module("adapters").capabilities
local sdk = require("gator").module("extensions").adapter_sdk

local supported = { available = true, modes = { "native" } }
local contract = capabilities.new({
	provider = "fixture",
	transport = supported,
	auth = supported,
	session = supported,
	permission = supported,
	model = supported,
	command = supported,
	tool = supported,
	context = supported,
	usage = supported,
})
local emitted
local adapter = sdk.define({
	name = "fixture",
	capabilities = contract,
	emit = function(event)
		emitted = event
	end,
})
adapter.emit({ provider = "fixture", type = "message", payload = { text = "fixture" } })
assert(
	adapter.capabilities.provider == "fixture" and emitted.type == "message" and emitted.payload.text == "fixture",
	"adapter SDK must expose stable capability and event contracts"
)
assert(
	not pcall(adapter.emit, { provider = "fixture", type = "message", payload = { token = "secret" } }),
	"adapter SDK events must reject credentials"
)
assert(
	not pcall(sdk.define, { name = "other", capabilities = contract, emit = function() end }),
	"adapter SDK must reject mismatched capability ownership"
)
