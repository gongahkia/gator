local runtime = require("gator").module("core").runtime

local ticks = { 7, 8 }
local value = runtime.new({
	clock = function()
		return table.remove(ticks, 1)
	end,
	identifier = function(prefix, sequence)
		return prefix .. "-fixture-" .. sequence
	end,
})
assert(
	runtime.is(value)
		and value:now() == 7
		and value:next_id("provider-run") == "provider-run-fixture-1"
		and value:now() == 8
		and value:next_id("provider-run") == "provider-run-fixture-2",
	"runtime services must use injected deterministic clocks and identifiers"
)
local invalid_clock = runtime.new({
	clock = function()
		return -1
	end,
})
local invalid_identifier = runtime.new({
	identifier = function()
		return "Provider"
	end,
})
assert(
	not pcall(invalid_clock.now, invalid_clock)
		and not pcall(invalid_identifier.next_id, invalid_identifier, "provider"),
	"runtime services must reject invalid clock and identifier results explicitly"
)
local failed_clock = runtime.new({
	clock = function()
		error("token: private-value")
	end,
})
local ok, failure = pcall(failed_clock.now, failed_clock)
assert(not ok and failure:find("private%-value") == nil, "runtime callback failures must redact diagnostic values")

local lifecycle_ticks = { 10, 11, 12, 13 }
local owned = runtime.new({
	clock = function()
		return table.remove(lifecycle_ticks, 1)
	end,
})
local starts, stops = {}, {}
owned:register("launch-queue", {
	start = function(_, opts)
		starts[#starts + 1] = opts
		return { id = "launch-queue" }
	end,
	stop = function(_, handle, reason)
		stops[#stops + 1] = { handle = handle, reason = reason }
	end,
})
owned:register("event-ingest", {
	start = function()
		return { id = "event-ingest" }
	end,
	stop = function(_, _, reason)
		stops[#stops + 1] = { reason = reason }
	end,
})
assert(
	owned:start("launch-queue", { limit = 1 }).state == "running"
		and owned:start("launch-queue").started_at == 10
		and #starts == 1
		and owned:stop("launch-queue", "cancelled").state == "cancelled"
		and stops[1].reason == "cancelled",
	"runtime owners must start services once and route explicit cancellation to their owner"
)
owned:start("event-ingest")
local shutdown = owned:shutdown()
assert(
	#shutdown == 1 and owned:status("event-ingest").state == "stopped" and stops[2].reason == "shutdown",
	"runtime owners must stop active services during shutdown"
)
local failing = runtime.new()
failing:register("broken-service", {
	start = function()
		error("token: private-value")
	end,
	stop = function() end,
})
ok, failure = pcall(failing.start, failing, "broken-service")
assert(
	not ok and failure:find("private%-value") == nil and failing:status("broken-service").state == "failed",
	"runtime owners must expose redacted startup failures"
)
assert(
	not pcall(owned.register, owned, "launch-queue", { start = function() end, stop = function() end })
		and owned:stop("event-ingest") == false,
	"runtime owners must reject duplicate services and inert cancellation"
)
