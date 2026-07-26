local health = require("gator.health")

local catalog = health.launch_catalog({
	cwd = vim.g.gator_test.root,
	executable = function(name)
		return name == "claude" or name == "codex" or name == "opencode" or name == "pi"
	end,
	run = function(argv, _, input)
		if argv[1] == "claude" and argv[2] == "--version" then
			return { code = 0, stdout = "2.1.119 (Claude Code)" }
		end
		if argv[1] == "claude" and argv[2] == "--help" then
			return { code = 0, stdout = "--resume" }
		end
		if argv[1] == "claude" and argv[2] == "auth" then
			return { code = 0, stdout = [[{"loggedIn":true}]] }
		end
		if argv[1] == "codex" and argv[2] == "--version" then
			return { code = 0, stdout = "codex-cli 0.144.4" }
		end
		if argv[1] == "codex" and argv[2] == "app-server" then
			return { code = 0, stdout = "--listen stdio://" }
		end
		if argv[1] == "codex" and argv[2] == "login" then
			return { code = 0, stdout = "Logged in" }
		end
		if argv[1] == "opencode" and argv[2] == "--version" then
			return { code = 0, stdout = "1.18.0" }
		end
		if argv[1] == "opencode" and argv[2] == "acp" then
			assert(input:find('"jsonrpc":"2.0"', 1, true), "OpenCode catalog must use JSON-RPC ACP initialization")
			return {
				code = 0,
				stdout = [[{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"agentCapabilities":{"sessionCapabilities":{"resume":{}}}}}]],
			}
		end
		if argv[1] == "opencode" and argv[2] == "providers" then
			return { code = 0, stdout = "1 credential" }
		end
		if argv[1] == "pi" and argv[2] == "--version" then
			return { code = 0, stdout = "0.82.0" }
		end
		if argv[1] == "pi" and argv[2] == "--help" then
			return { code = 0, stdout = "--mode rpc --session --session-id --tools --exclude-tools" }
		end
		if argv[1] == "pi" and argv[2] == "--mode" then
			return {
				code = 0,
				stdout = [[{"id":"gator-probe","type":"response","command":"get_state","success":true,"data":{"sessionFile":"pi-session"}}]],
			}
		end
		error("unexpected provider probe")
	end,
	pi_user_confirmed = true,
})

local ready = {}
for _, record in ipairs(catalog) do
	ready[record.provider] = record.available
end
assert(
	ready.claude and ready.codex and ready.opencode and ready.pi,
	"native launch catalog must expose verified providers and explicitly user-confirmed Pi"
)
local unconfirmed = health.launch_catalog({
	cwd = vim.g.gator_test.root,
	executable = function(name)
		return name == "pi"
	end,
	run = function(argv, _, input)
		if argv[2] == "--version" then
			return { code = 0, stdout = "0.82.0" }
		end
		if argv[2] == "--help" then
			return { code = 0, stdout = "--mode rpc --session --session-id --tools --exclude-tools" }
		end
		assert(argv[2] == "--mode" and input:find('"get_state"', 1, true), "Pi must retain its credential-free probe")
		return {
			code = 0,
			stdout = [[{"id":"gator-probe","type":"response","command":"get_state","success":true,"data":{"sessionFile":"pi-session"}}]],
		}
	end,
})
local pi
for _, record in ipairs(unconfirmed) do
	if record.provider == "pi" then
		pi = record
	end
end
assert(
	pi
		and not pi.available
		and pi.readiness_state == "detected"
		and pi.reason:find("providers.pi.user_confirmed", 1, true),
	"Pi must remain unavailable without explicit local user confirmation"
)
