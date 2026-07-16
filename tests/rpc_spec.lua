local rpc = require("gator").module("adapters").rpc
local output, notices, clock = {}, {}, 0
local transport = rpc.new({
	write = function(value)
		table.insert(output, value)
	end,
	on_notification = function(name, params)
		table.insert(notices, { name = name, params = params })
	end,
	now = function()
		return clock
	end,
})
local response
local id = transport:request("session/start", { workspace = "project" }, function(result, error)
	response = { result = result, error = error }
end, 10)
assert(id == 1 and output[1]:find("session/start", 1, true), "requests must use framed JSON-RPC")
transport:feed('{"jsonrpc":"2.0","id":1,"result":{"session":"native-one"}')
assert(not response, "partial frames must not dispatch")
transport:feed("}\n")
assert(
	response.result.session == "native-one" and response.error == nil,
	"responses must route to their request callback"
)
transport:feed('{"jsonrpc":"2.0","method":"session/update","params":{"state":"running"}}\n')
assert(notices[1].name == "session/update", "notifications must dispatch independently")
local cancelled
local cancelled_id = transport:request("session/stop", {}, function(_, error)
	cancelled = error
end, 10)
transport:cancel(cancelled_id)
assert(cancelled.code == -32800 and output[#output]:find("$/cancelRequest", 1, true), "cancellation must be explicit")
transport:request("slow", {}, function(_, error)
	response = { error = error }
end, 1)
clock = 1
assert(transport:expire() == 1 and response.error.code == -32001, "expired requests must fail explicitly")
