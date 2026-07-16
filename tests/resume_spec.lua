local adapters = require("gator").module("adapters")
local core = require("gator").module("core")
local resume = adapters.resume
local function contract(session)
	local supported = { available = true, modes = { "native" } }
	return adapters.capabilities.new({
		provider = "codex",
		transport = supported,
		auth = supported,
		session = session,
		permission = supported,
		model = supported,
		command = supported,
		tool = supported,
		context = supported,
		usage = supported,
	})
end
local linked = core.session.new({ task_id = "task-resume", provider = "codex", id = "native-one", owner = "provider" })
local resumed
local native = resume.session({
	session = linked,
	capabilities = contract({ available = true, modes = { "resume" } }),
	native_resume = function(reference)
		resumed = reference
	end,
})
assert(native.status == "resumed" and resumed.id == "native-one", "supported sessions must resume natively")
local orphan = core.session_metadata.new({
	task_id = "task-resume",
	provider = "codex",
	id = "native-one",
	owner = "provider",
	provider_version = "1",
	capabilities = { resume = false },
	probed_at = 1,
})
local saved
local fallback = resume.session({
	session = linked,
	capabilities = contract({ available = false, reason = "resume unsupported" }),
	metadata = orphan,
	persist_orphan = function(record)
		saved = record
	end,
})
assert(fallback.status == "orphaned" and saved.id == "native-one", "unsupported resume must preserve orphan metadata")
local ok = pcall(
	resume.session,
	{ session = linked, capabilities = contract({ available = false, reason = "resume unsupported" }) }
)
assert(not ok, "unsupported resume must not silently create a replacement session")
ok = pcall(resume.session, {
	session = linked,
	capabilities = contract({ available = true, modes = { "resume" } }),
	token = "secret",
})
assert(not ok, "resume must reject credential fields")
