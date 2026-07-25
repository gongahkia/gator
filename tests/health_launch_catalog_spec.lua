local health = require("gator.health")

local catalog = health.launch_catalog({
	cwd = vim.g.gator_test.root,
	executable = function(name)
		return name == "claude" or name == "codex" or name == "opencode"
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
		error("unexpected provider probe")
	end,
})

local ready = {}
for _, record in ipairs(catalog) do
	ready[record.provider] = record.available
end
assert(
	ready.claude and ready.codex and ready.opencode,
	"native launch catalog must expose only providers with verified executable, support, authentication, and bridge capability"
)
