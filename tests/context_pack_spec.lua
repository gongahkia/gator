local pack = require("gator").module("context").pack
local value = pack.new({
	id = "pack-one",
	task_id = "task-context",
	entries = {
		{
			id = "entry-file",
			kind = "file",
			ref = "lua/gator/init.lua",
			provenance = { source = "buffer", ref = "buf:1" },
			trust = "provenance",
			token_estimate = { status = "estimated", tokens = 120 },
			transfer = { eligible = true },
		},
		{
			id = "entry-diagnostic",
			kind = "diagnostic",
			ref = "diag:1",
			provenance = { source = "lsp", ref = "diag:1" },
			trust = "manual",
			token_estimate = { status = "unavailable", reason = "provider does not expose token accounting" },
			transfer = { eligible = false, reason = "manual confirmation is required" },
		},
	},
})

assert(pack.is(value), "context packs must have a distinct entity type")
assert(
	value.entries[1].id == "entry-file" and value.entries[2].id == "entry-diagnostic",
	"context entries must preserve order"
)
assert(value.entries[1].provenance.source == "buffer", "context entries must preserve provenance")
assert(value.entries[2].trust == "manual", "context entries must preserve trust")
assert(value.entries[2].token_estimate.status == "unavailable", "context packs must preserve unavailable estimates")
assert(not value.entries[2].transfer.eligible, "context packs must preserve transfer ineligibility")

local record = pack.to_record(value)
record.entries[1].ref = "modified"
assert(value.entries[1].ref == "lua/gator/init.lua", "context records must not mutate packs")
assert(pack.from_record(pack.to_record(value)).id == "pack-one", "context packs must round-trip")

local ok = pcall(pack.entry, {
	id = "entry-invalid",
	kind = "file",
	ref = "file",
	provenance = { source = "buffer", ref = "buf:1" },
	trust = "provenance",
	token_estimate = { status = "estimated", tokens = -1 },
	transfer = { eligible = true },
})
assert(not ok, "invalid token estimates must fail explicitly")
ok = pcall(pack.entry, {
	id = "entry-secret",
	kind = "file",
	ref = "file",
	provenance = { source = "buffer", ref = "buf:1", token = "credential" },
	trust = "provenance",
	token_estimate = { status = "estimated", tokens = 1 },
	transfer = { eligible = true },
})
assert(not ok, "context provenance must reject credential fields")
