local extensions = require("gator").module("extensions").manager
local actions = require("gator.ui.actions")

package.preload.gator_test_extension = function()
	return function(api)
		api.actions.register({
			name = "extension-action",
			label = "Extension action",
			execute = function()
				return "loaded"
			end,
		})
		api.health.register("ready", function(report)
			report.ok("fixture ready")
		end)
		api.events.on("run.created", function() end)
		api.providers.register({
			name = "fixture-terminal",
			kind = "terminal",
			probe = function()
				return { available = true, version = "1" }
			end,
			start = function()
				return { session = { id = "fixture-session" }, command = { "fixture" } }
			end,
		})
		api.ui.renderer({
			id = "fixture-picker",
			slot = "provider_picker",
			render = function()
				return true
			end,
		})
		api.ui.column({
			id = "fixture-column",
			label = "Fixture",
			render = function()
				return "value"
			end,
		})
		api.context.collector({
			name = "fixture-context",
			collect = function()
				return { text = "extra context" }
			end,
		})
		api.context.redactor({
			name = "fixture-redactor",
			redact = function(value)
				return value:gsub("private", "[REDACTED]")
			end,
		})
		api.context.handoff_formatter({
			name = "fixture-handoff",
			format = function()
				return "handoff detail"
			end,
		})
	end
end

local manager = extensions.open({
	modules = { "gator_test_extension" },
	renderers = { provider_picker = "fixture-picker" },
	columns = { "fixture-column" },
})
local status = manager:load()
assert(status[1].state == "ready", "explicit modules must load without directory discovery")
assert(actions.list()[1].execute() == "loaded", "loaded extensions must register workspace actions")
assert(manager:provider("fixture-terminal").kind == "terminal", "extensions must register terminal providers")
assert(manager:render("provider_picker", { actions = {} }), "configured core renderer must be selected")
assert(manager:render_column("fixture-column", {}) == "value", "configured graph columns must render")
assert(manager:collect({})[1].text == "extra context", "context collectors must contribute artifacts")
assert(manager:redact("private value") == "[REDACTED] value", "context redactors must run before delivery")
assert(manager:format_handoff({})[1].text == "handoff detail", "handoff formatters must contribute sections")
manager:close()
assert(#actions.list() == 0, "extension close must remove workspace actions")

package.preload.gator_test_failure = function()
	return function(api)
		api.actions.register({ name = "failing-extension-action", execute = function() end })
		api.events.on("run.created", function()
			error("fixture failure")
		end)
	end
end
local failing = extensions.open({ modules = { "gator_test_failure" } })
failing:load()
local notify = vim.notify
vim.notify = function() end
failing:emit("run.created", { run_id = "run-one" })
vim.notify = notify
assert(failing:status()[1].state == "disabled", "failing lifecycle callbacks must disable only their extension")
assert(#actions.list() == 0, "disabled extensions must unregister their resources")
package.preload.gator_test_extension = nil
package.preload.gator_test_failure = nil
