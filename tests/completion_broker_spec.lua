local broker = require("gator.completion.broker")
local config = require("gator.config")

local writes, callbacks, signals = {}, {}, {}
local settings = config.resolve({ completion = { sidecar = { argv = { "fixture-sidecar" } } } }).completion
local value = broker.new({
	settings = settings,
	spawn = function(_, _, stdout, _, on_exit)
		callbacks.stdout, callbacks.exit = stdout, on_exit
		return {
			pid = 41,
			write = function(_, text)
				table.insert(writes, text)
				return true
			end,
			kill = function(_, signal)
				table.insert(signals, signal)
				return true
			end,
		}
	end,
})
local received
local ticket = value:request({ workspace = { root = vim.fn.getcwd() } }, function(items, error)
	received = { items = items, error = error }
end)
assert(ticket and writes[1]:find('"method":"gator/completion"', 1, true), "completion requests must use JSON-RPC")
callbacks.stdout(nil, '{"jsonrpc":"2.0","id":1,"result":{"items":[{"text":"value"}]}}\n')
assert(received.items[1].text == "value" and not received.error, "valid sidecar responses must reach callers")
assert(value:accept(ticket.root, "candidate-one"), "accepted candidates must be reported to the sidecar")
assert(
	writes[#writes]:find("gator/completionAccepted", 1, true),
	"acceptance notifications must not contain suggestion text"
)

local cancelled
local second = value:request({ workspace = { root = vim.fn.getcwd() } }, function(_, error)
	cancelled = error
end)
assert(value:cancel(second) and cancelled.code == "sidecar", "sidecar cancellation must settle callers")
assert(writes[#writes]:find("$/cancelRequest", 1, true), "cancellation must send JSON-RPC cancellation")

value:close()
assert(signals[1] == 15, "closing completion must terminate persistent sidecars")

local unconfigured = broker.new({ settings = config.resolve().completion })
local unavailable
assert(not unconfigured:request({ workspace = { root = vim.fn.getcwd() } }, function(_, error)
	unavailable = error
end), "unconfigured completion must not launch a sidecar")
assert(unavailable.code == "unconfigured", "unconfigured completion must report a typed error")
