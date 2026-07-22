local review = require("gator").module("context").handoff_pack_review
local handoff_pack = require("gator").module("context").handoff_pack
local pack = require("gator").module("context").pack

local function entry(id)
	return pack.entry({
		id = id,
		kind = "file",
		ref = "file://" .. id,
		provenance = { source = "fixture", ref = id },
		trust = "manual",
		token_estimate = { status = "estimated", tokens = 1 },
		transfer = { eligible = true },
	})
end

local source = handoff_pack.new({
	id = "pack-review",
	task_id = "task-review",
	entries = { entry("entry-one"), entry("entry-two") },
})
local value = review.new({ pack = source })
assert(review.is(value) and value:status().state == "ready", "handoff pack reviews must begin ready")
value:append(entry("entry-three"))
value:annotate("entry-three", "review token=fixture-secret")
value:move("entry-three", 1)
local removed = value:remove("entry-two")
assert(
	removed.id == "entry-two"
		and vim.deep_equal(
			vim.tbl_map(function(item)
				return item.id
			end, value:status().entries),
			{ "entry-three", "entry-one" }
		)
		and value:status().entries[1].annotation:find("fixture-secret", 1, true) == nil,
	"handoff reviews must support ordered add, annotate, move, and remove operations with redaction"
)
local committed = value:commit()
assert(
	handoff_pack.is(committed)
		and committed.entries[1].id == "entry-three"
		and source.entries[1].id == "entry-one"
		and value:status().state == "completed",
	"handoff review commit must create an immutable edited pack without mutating its source"
)
assert(
	not value:commit() and not pcall(value.append, value, entry("entry-four")),
	"completed handoff reviews must reject further edits"
)

local cancelled = review.new({ pack = source })
assert(
	cancelled:cancel() and cancelled:status().state == "cancelled" and not cancelled:commit(),
	"handoff reviews must cancel explicitly"
)
assert(not pcall(review.new, { pack = {} }) and not pcall(function()
	review.new({ pack = source }):move("entry-one", 3)
end), "handoff reviews must reject invalid source packs and destinations")
