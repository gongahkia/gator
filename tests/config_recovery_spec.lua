local gator = require("gator")

local active = gator.setup()
local previous = active._coordinator
local operation = active._coordinator:start_operation({ id = "config-recovery" })
local ok, diagnostic = pcall(gator.setup, { telemetry = { token = "private-value" } })
assert(
	not ok and diagnostic:find("private%-value") == nil and gator._coordinator == previous and operation:status().active,
	"invalid configuration must fail redacted without cancelling an active configuration"
)

local recovered = gator.setup({ ui = { layout = "modal" } })
assert(
	recovered._coordinator ~= previous
		and recovered._state.config.ui.layout == "modal"
		and recovered._coordinator:state().compatibility.supported
		and operation:status().cancelled,
	"valid configuration must replace the active state after an invalid configuration failure"
)
