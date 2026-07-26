local config = require("gator.config")
local state = require("gator.state")
local workflow = require("gator.workflow")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("workflow-send-context")
assert(vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0, "fixture must initialize Git")
helpers.write(root .. "/source.lua", "local value = 1\nreturn value\n")
vim.cmd("enew!")
local buffer = vim.api.nvim_get_current_buf()
vim.api.nvim_buf_set_lines(buffer, 0, -1, false, { "local value = 1", "return value" })

local value = workflow.new({
	state = state.new(config.resolve({ providers = { pi = { user_confirmed = true } } }), { supported = true }),
	root = root,
	readiness = function()
		return { { provider = "pi", available = true, readiness_state = "user_confirmed" } }
	end,
})
value:put({
	id = "run-context",
	provider = "pi",
	role = "primary",
	transport = "chat",
	state = "waiting_input",
	workspace = { kind = "project", root = root },
	bundle_id = "bundle-context",
	objective = "Inspect selected source",
	transcript = "available",
	usage = { state = "unknown", context_tokens_estimate = 1 },
	budget = { limit_tokens = 0, action = "warn", state = "unbounded" },
	session = { id = "pi-session", resume_supported = true },
	created_at = 1,
	updated_at = 1,
})
local sent = {}
value.active["run-context"] = {
	kind = "structured",
	handle = {
		send = function(message)
			table.insert(sent, message)
			return true
		end,
	},
}

assert(
	value:send_context({ run_id = "run-context", kind = "selection", buffer = buffer, first_line = 1, last_line = 1 }),
	"active structured chats must accept provenance-labelled selections"
)
assert(
	#sent == 1
		and sent[1]:find("Kind: selection", 1, true)
		and sent[1]:find("local value = 1", 1, true)
		and (value:transcript(value:run("run-context")) or ""):find("Gator follow-up editor context", 1, true),
	"editor context must become a persisted Gator-owned follow-up turn"
)

value.store:bundle("bundle-context", "# Named context\n\nUse this exact artifact.")
assert(
	value:send_context({ run_id = "run-context", kind = "bundle", bundle_id = "bundle-context" }),
	"named bundles must be sendable"
)
assert(sent[#sent]:find("Kind: named bundle", 1, true), "named bundle sends must identify their provenance")

value:put({
	id = "run-terminal",
	provider = "pi",
	role = "primary",
	transport = "terminal",
	state = "running",
	workspace = { kind = "project", root = root },
	bundle_id = "bundle-terminal",
	objective = "Terminal",
	transcript = "unavailable",
	usage = { state = "unknown", context_tokens_estimate = 1 },
	budget = { limit_tokens = 0, action = "warn", state = "unbounded" },
	session = { id = "pi-terminal", resume_supported = true },
	created_at = 1,
	updated_at = 1,
})
value.active["run-terminal"] = { kind = "terminal", terminal_id = "run-terminal" }
assert(
	not pcall(value.send_context, value, { run_id = "run-terminal", kind = "selection", buffer = buffer }),
	"editor context must never be injected into native terminal jobs"
)
