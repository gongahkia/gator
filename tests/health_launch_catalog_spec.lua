local health = require("gator.health")

local catalog = health.launch_catalog({
	cwd = vim.g.gator_test.root,
	executable = function(name)
		return name == "codex" or name == "pi"
	end,
	run = function(argv, _, input)
		if argv[1] == "codex" and argv[2] == "--version" then
			return { code = 0, stdout = "codex-cli 0.145.0" }
		end
		if argv[1] == "codex" and argv[2] == "app-server" then
			return { code = 0, stdout = "--listen stdio://" }
		end
		if argv[1] == "codex" and argv[2] == "login" then
			return { code = 0, stdout = "Logged in" }
		end
		if argv[1] == "pi" and argv[2] == "--version" then
			return { code = 0, stdout = "0.82.1" }
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
	ready.codex and ready.pi and ready.claude == nil and ready.opencode == nil,
	"native launch catalog must expose only supported native terminal providers"
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
