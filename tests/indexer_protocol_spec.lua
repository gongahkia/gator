local protocol = require("gator").module("indexer").protocol
local request = protocol.request("request-one", "cancel", { request_id = "index-one" })
assert(request.version == 1 and request.method == "cancel", "Lua indexer requests must be versioned")
assert(
	protocol.response({ version = 1, id = "request-one", ok = true, result = { cancelled = "index-one" } }).ok,
	"Lua indexer responses must preserve successful cancellations"
)
assert(not pcall(protocol.response, { version = 1, ok = false }), "protocol errors must require explicit messages")
