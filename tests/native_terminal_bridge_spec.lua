local bridge = require("gator.adapters.native_terminal").new({
	uuid = function()
		return "12345678-1234-4123-8123-123456789012"
	end,
})

local created
bridge:start({ provider = "claude", cwd = vim.g.gator_test.root, prompt = "Review this task" }, function(value, reason)
	assert(reason == nil, "Claude terminal bridge must create a session without an RPC failure")
	created = value
end)
assert(
	created.session.id == "12345678-1234-4123-8123-123456789012"
		and vim.deep_equal(created.command, { "claude", "--session-id", created.session.id, "Review this task" }),
	"Claude terminal launch must use an explicit provider-owned session id and initial prompt"
)

local resumed
bridge:resume({ provider = "claude", session = created.session }, function(value, reason)
	assert(reason == nil, "Claude terminal bridge must resume a known provider session")
	resumed = value
end)
assert(
	vim.deep_equal(resumed.command, { "claude", "--resume", created.session.id }),
	"Claude terminal resume must preserve the exact provider session id"
)
local pi
bridge:start({ provider = "pi", cwd = vim.g.gator_test.root, prompt = "Review Pi" }, function(value, reason)
	assert(reason == nil, "Pi terminal bridge must create a provider-owned session id")
	pi = value
end)
assert(
	vim.deep_equal(pi.command, { "pi", "--session-id", pi.session.id, "Review Pi" }),
	"Pi launch must create the provider session before opening its native terminal"
)
local pi_resumed
bridge:resume({ provider = "pi", session = pi.session }, function(value, reason)
	assert(reason == nil, "Pi terminal bridge must resume a known provider session")
	pi_resumed = value
end)
assert(
	vim.deep_equal(pi_resumed.command, { "pi", "--session", pi.session.id }),
	"Pi resume must preserve the exact provider session id"
)
assert(
	not pcall(
		bridge.start,
		bridge,
		{ provider = "gemini", cwd = vim.g.gator_test.root, prompt = "No bridge" },
		function() end
	),
	"providers without a verified native terminal bridge must fail closed"
)

local requests, spawned = {}, {}
local function spawn(command, opts)
	table.insert(spawned, { command = command, cwd = opts.cwd })
	local process = {}
	function process:write(frame)
		local request = vim.json.decode(frame)
		table.insert(requests, request)
		if request.id == nil then
			return true
		end
		local result
		if request.method == "thread/start" then
			result = { thread = { id = "codex-thread" } }
		elseif request.method == "session/new" then
			result = { sessionId = "opencode-session" }
		else
			result = {}
		end
		vim.schedule(function()
			opts.stdout(nil, vim.json.encode({ id = request.id, result = result }) .. "\n")
		end)
		return true
	end
	function process:kill()
		return true
	end
	return process
end
local rpc_bridge = require("gator.adapters.native_terminal").new({ spawn = spawn })
local codex
rpc_bridge:start({ provider = "codex", cwd = vim.g.gator_test.root, prompt = "Review Codex" }, function(value, reason)
	assert(reason == nil, "Codex terminal bridge must create a recoverable provider thread")
	codex = value
end)
assert(
	vim.wait(1000, function()
		return codex ~= nil
	end)
		and vim.deep_equal(spawned[1].command, { "codex", "app-server" })
		and requests[2].method == "initialized"
		and requests[3].method == "thread/start"
		and requests[3].params.ephemeral == false
		and vim.deep_equal(codex.command, { "codex", "resume", "codex-thread", "Review Codex" }),
	"Codex launch must create a persistent app-server thread before opening its native terminal"
)
local opencode
rpc_bridge:start(
	{ provider = "opencode", cwd = vim.g.gator_test.root, prompt = "Review OpenCode" },
	function(value, reason)
		assert(reason == nil, "OpenCode terminal bridge must create an ACP session")
		opencode = value
	end
)
assert(
	vim.wait(1000, function()
		return opencode ~= nil
	end)
		and vim.deep_equal(spawned[2].command, { "opencode", "acp", "--cwd", vim.g.gator_test.root })
		and requests[4].jsonrpc == "2.0"
		and requests[5].jsonrpc == "2.0"
		and requests[5].method == "session/new"
		and vim.deep_equal(
			opencode.command,
			{ "opencode", "--session", "opencode-session", "--prompt", "Review OpenCode" }
		),
	"OpenCode launch must negotiate ACP and preserve the provider-owned session id in its terminal command"
)

local terminal_safe = false
local terminal_timer
local callback_bridge = require("gator.adapters.native_terminal").new({
	spawn = function(_, opts)
		return {
			write = function(_, frame)
				local request = vim.json.decode(frame)
				if request.id == nil then
					return true
				end
				local result = request.method == "thread/start" and { thread = { id = "fast-thread" } } or {}
				terminal_timer = vim.uv.new_timer()
				terminal_timer:start(0, 0, function()
					terminal_timer:stop()
					terminal_timer:close()
					opts.stdout(nil, vim.json.encode({ id = request.id, result = result }) .. "\n")
				end)
				return true
			end,
			kill = function()
				return true
			end,
		}
	end,
})
callback_bridge:start(
	{ provider = "codex", cwd = vim.g.gator_test.root, prompt = "Check callback context" },
	function(value)
		local checked = vim.system({ "git", "rev-parse", "--is-inside-work-tree" }, { text = true }):wait()
		terminal_safe = value.session.id == "fast-thread" and checked.code == 0
	end
)
assert(
	vim.wait(1000, function()
		return terminal_safe
	end),
	"terminal bridge callbacks must schedule lifecycle work outside fast-event context"
)
