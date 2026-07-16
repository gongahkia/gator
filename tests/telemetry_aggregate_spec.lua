local aggregate = require("gator").module("telemetry").aggregate
local consent = require("gator").module("telemetry").consent
local disabled = aggregate.new({ consent = consent.new({ enabled = false }) })
assert(not pcall(disabled.record, disabled, {
	schema_version = 1,
	type = "feature",
	at = 1,
	fields = { feature = "dashboard" },
}), "telemetry aggregation must require explicit collection consent")
local value = aggregate.new({ consent = consent.new({ enabled = true }) })
for _, attrs in ipairs({
	{ schema_version = 1, type = "feature", at = 1, fields = { feature = "dashboard" } },
	{ schema_version = 1, type = "feature", at = 2, fields = { feature = "dashboard" } },
	{
		schema_version = 1,
		type = "error",
		at = 3,
		fields = { code = "indexer.failed", detail = "secret=aggregate-secret /Users/example/project" },
	},
	{ schema_version = 1, type = "performance", at = 4, fields = { operation = "context.search", duration_ms = 12 } },
	{ schema_version = 1, type = "health", at = 5, fields = { component = "indexer", status = "degraded" } },
}) do
	assert(value:record(attrs), "consented telemetry events must aggregate")
end
local snapshot = value:snapshot()
assert(
	snapshot.features[1].feature == "dashboard"
		and snapshot.features[1].count == 2
		and snapshot.errors[1].code == "indexer.failed"
		and snapshot.performance[1].duration_ms == 12
		and snapshot.health[1].status == "degraded",
	"anonymous telemetry aggregation must retain only aggregate product metrics"
)
local output = vim.json.encode(snapshot)
assert(
	not output:find("aggregate%-secret") and not output:find("/Users/example/project", 1, true),
	"anonymous telemetry aggregates must exclude transcript, code, and path content"
)
