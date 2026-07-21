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
