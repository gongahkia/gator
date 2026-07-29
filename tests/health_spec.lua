local health = require("gator.health")
local events = {}

health.register("provider.fixture", function(report)
	report.ok("fixture provider is ready")
end)
health.register("provider.failure", function()
	error("fixture provider failed")
end)

local reporter = {}
for _, method in ipairs({ "start", "ok", "warn", "error" }) do
	reporter[method] = function(message)
		table.insert(events, { method = method, message = message })
	end
end

health.run(reporter)

local seen = {}
for _, event in ipairs(events) do
	seen[event.method .. ":" .. event.message] = true
end
assert(seen["start:Gator provider.fixture"], "registered provider checks must run")
assert(seen["ok:fixture provider is ready"], "provider checks must report through the shared reporter")
assert(seen["start:Gator provider.failure"], "failing provider checks must run")
local failure_reported = false
for _, event in ipairs(events) do
	if event.method == "error" and event.message:find("Health check failed: provider.failure", 1, true) then
		failure_reported = true
	end
end
assert(failure_reported, "failing checks must report explicit health errors")

local original_readiness = health.readiness
health.readiness = function()
	return {
		{ component = "adapter.codex", level = "ok", message = "Codex is ready" },
		{ component = "adapter.aider", level = "warn", message = "Aider needs setup", repair = "set up Aider" },
		{ component = "git", level = "ok", message = "Git is ready" },
	}
end
local compact = {}
for _, method in ipairs({ "start", "ok", "warn", "error" }) do
	reporter[method] = function(message)
		table.insert(compact, { method = method, message = message })
	end
end
health.run(reporter)
local compact_output = {}
for _, event in ipairs(compact) do
	table.insert(compact_output, event.message)
end
local compact_text = table.concat(compact_output, "\n")
assert(
	compact_text:find("Ready now: codex", 1, true)
		and compact_text:find("1 adapter(s) need setup or verification: aider", 1, true)
		and not compact_text:find("Aider needs setup", 1, true),
	"compact health must lead with ready agents and collapse optional adapter warnings"
)

local verbose = {}
for _, method in ipairs({ "start", "ok", "warn", "error" }) do
	reporter[method] = function(message)
		table.insert(verbose, message)
	end
end
health.run(reporter, { verbose = true })
assert(
	table.concat(verbose, "\n"):find("Aider needs setup", 1, true),
	"verbose health must retain individual provider diagnostics"
)
health.readiness = original_readiness

local original_health = vim.health
local before = #events
vim.health = reporter
health.check()
vim.health = original_health
assert(#events > before, "checkhealth entry point must use the shared health framework")

local ok = pcall(health.register, "provider.fixture", function() end)
assert(not ok, "duplicate health checks must fail explicitly")
ok = pcall(health.register, "Provider", function() end)
assert(not ok, "invalid health check names must fail explicitly")
