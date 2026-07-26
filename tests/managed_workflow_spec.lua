local config = require("gator.config")
local state = require("gator.state")
local capabilities = require("gator.adapters.capabilities")
local managed_adapter = require("gator.adapters.managed")
local workflow = require("gator.workflow")
local ui = require("gator.ui")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("managed-workflow")
local original_catalog = managed_adapter.catalog
managed_adapter.catalog = function()
	return { { provider = "gemini", mode = "acp", available = true, readiness_state = "user_confirmed" } }
end

local runtime = { opened = {}, sent = {}, cancelled = 0, stopped = 0, shutdowns = 0, active = true }
function runtime:open(opts)
	table.insert(self.opened, opts)
	opts.on_session({ provider = opts.provider, id = "managed-session", owner = "provider", mode = "acp" })
	return { id = "managed-session" }
end
function runtime:is_active()
	return self.active
end
function runtime:send(reference, prompt)
	table.insert(self.sent, { reference = reference, prompt = prompt })
	return true
end
function runtime:cancel()
	self.cancelled = self.cancelled + 1
	return true
end
function runtime:stop()
	self.stopped = self.stopped + 1
	self.active = false
	return true
end
function runtime:shutdown()
	self.shutdowns = self.shutdowns + 1
	return self.stopped
end

local current = state.new(config.resolve({ providers = { gemini = { user_confirmed = true } } }), { supported = true })
local value = workflow.new({
	state = current,
	root = root,
	managed = runtime,
	readiness = function()
		return {}
	end,
})
local created = value:create("Managed provider task")
assert(
	capabilities.supports(value.providers.gemini, "transport", "managed")
		and capabilities.supports(value.providers.gemini, "permission", "user_decision"),
	"managed ACP providers must advertise Gator transport and explicit approval support"
)
value:launch("gemini")
local running = value:task(created.id)
assert(
	#runtime.opened == 1
		and runtime.opened[1].prompt == created.objective
		and running.lifecycle == "running"
		and running.sessions[1].mode == "acp",
	"managed launch must persist and run the ACP provider session"
)
assert(value:prompt_session({ task_id = created.id, provider = "gemini", id = "managed-session" }, "continue"))
assert(runtime.sent[1].prompt == "continue", "managed session input must route to the active provider process")

local decision
runtime.opened[1].on_permission({
	provider = "gemini",
	session_id = "managed-session",
	request_id = "request-one",
	action = "session/request_permission",
	details = { operation = "write" },
}, function(value)
	decision = value
	return true
end)
assert(
	ui.approval_details.approve() and decision == "approved",
	"managed ACP permissions must route through Gator approval UI"
)

runtime.opened[1].on_exit({ code = 0 })
assert(value:task(created.id).lifecycle == "awaiting_review", "completed managed runs must become review-ready")
runtime.active = false
assert(value:attach() and #runtime.opened == 2, "managed sessions must reopen from their persisted provider session id")
runtime.active = true
assert(
	value:stop_session()
		and runtime.stopped == 1
		and not runtime.active
		and value:task(created.id).lifecycle == "running",
	"stopping a managed child must preserve a resumable provider session and running task"
)
ui.conversation.close()
value:close()
assert(runtime.shutdowns == 1, "workflow disposal must stop managed children")
managed_adapter.catalog = original_catalog
