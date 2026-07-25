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
assert(
	not pcall(
		bridge.start,
		bridge,
		{ provider = "gemini", cwd = vim.g.gator_test.root, prompt = "No bridge" },
		function() end
	),
	"providers without a verified native terminal bridge must fail closed"
)
