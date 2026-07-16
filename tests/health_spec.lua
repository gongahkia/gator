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
