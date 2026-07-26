local config = require("gator.config")
local state = require("gator.state")
local managed_adapter = require("gator.adapters.managed")
local session = require("gator.core.session")
local workflow = require("gator.workflow")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local original_catalog = managed_adapter.catalog
managed_adapter.catalog = function()
	return { { provider = "copilot", mode = "acp", available = true, readiness_state = "user_confirmed" } }
end

local opened
local terminal = {
	open = function(_, opts)
		opened = opts
		return { id = opts.id }
	end,
}
local runtime = {}
function runtime:open(opts)
	opts.on_resume_fallback({
		provider = "copilot",
		session = { provider = "copilot", id = "copilot-session", owner = "provider", mode = "acp" },
		reason = "Copilot ACP session/load was rejected",
	})
	return { fallback = true }
end
function runtime:is_active()
	return false
end
function runtime:send()
	return true
end
function runtime:cancel()
	return true
end
function runtime:stop()
	return true
end
function runtime:shutdown()
	return 0
end

local value = workflow.new({
	state = state.new(config.resolve({ providers = { copilot = { user_confirmed = true } } }), { supported = true }),
	root = helpers.tempdir("copilot-resume"),
	managed = runtime,
	terminal = terminal,
	readiness = function()
		return {}
	end,
})
local task = value:create("Copilot resume fallback")
value:replace(session.link(
	value:task(task.id),
	session.new({
		task_id = task.id,
		provider = "copilot",
		id = "copilot-session",
		owner = "provider",
		mode = "acp",
	})
))
assert(
	value:attach()
		and vim.deep_equal(opened.command, { "copilot", "--resume", "copilot-session" })
		and opened.cwd == value:task(task.id).workspace.root,
	"Copilot ACP reattach rejection must open the documented interactive CLI resume fallback"
)
value:close()
managed_adapter.catalog = original_catalog
