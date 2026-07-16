local automatic = require("gator").module("context").automatic
local pack = require("gator").module("context").pack
local audit
local value = pack.new({
	id = "pack-auto",
	task_id = "task-auto",
	entries = {
		{
			id = "entry-allow",
			kind = "file",
			ref = "a",
			provenance = { source = "retrieval", ref = "a" },
			trust = "provenance",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
		},
		{
			id = "entry-block",
			kind = "file",
			ref = "b",
			provenance = { source = "retrieval", ref = "b" },
			trust = "provenance",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
		},
		{
			id = "entry-ineligible",
			kind = "file",
			ref = "c",
			provenance = { source = "retrieval", ref = "c" },
			trust = "provenance",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = false, reason = "restricted" },
		},
	},
})
local selected = automatic.attach({
	pack = value,
	policy = function(entry)
		return {
			allowed = entry.id ~= "entry-block",
			reason = entry.id == "entry-block" and "blocked by policy" or "approved by policy",
		}
	end,
	audit = function(value)
		audit = value
	end,
})
assert(
	#selected.entries == 1 and selected.entries[1].id == "entry-allow",
	"automatic mode must attach only approved eligible context"
)
assert(
	#audit == 3 and not audit[2].allowed and audit[3].reason == "restricted",
	"automatic mode must audit every retrieval result"
)
