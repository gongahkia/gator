local context_inspector = require("gator.ui").context_inspector
local pack = require("gator.context.pack")
local confirmed
local context_pack = pack.new({
	id = "pack-inspector",
	task_id = "task-inspector",
	entries = {
		{
			id = "entry-one",
			kind = "file",
			ref = "lua/gator/init.lua",
			provenance = { source = "repository", ref = "HEAD" },
			trust = "repository",
			token_estimate = { status = "estimated", tokens = 42 },
			transfer = { eligible = true },
		},
		{
			id = "entry-two",
			kind = "diagnostic",
			ref = "diagnostic://one",
			provenance = { source = "manual", ref = "operator" },
			trust = "manual",
			token_estimate = { status = "unavailable", reason = "not counted" },
			transfer = { eligible = false, reason = "contains restricted data" },
		},
	},
})

local window = context_inspector.open({
	pack = context_pack,
	on_confirm = function(selected)
		confirmed = selected
	end,
})

assert(vim.api.nvim_win_is_valid(window), "context inspector opening must create a window")
assert(not context_inspector.toggle("entry-one"), "eligible context entries must be excludable")
assert(context_inspector.toggle("entry-one"), "eligible context entries must be includable")
context_inspector.add({
	id = "entry-three",
	kind = "diff",
	ref = "diff://one",
	provenance = { source = "repository", ref = "HEAD" },
	trust = "repository",
	token_estimate = { status = "estimated", tokens = 7 },
	transfer = { eligible = true },
})
context_inspector.annotate("entry-three", "review first")
context_inspector.pin("entry-three", true)
context_inspector.move("entry-three", 1)
context_inspector.remove("entry-two")
local ok = pcall(context_inspector.toggle, "entry-two")
assert(not ok, "removed entries must remain unavailable")
local selected = context_inspector.confirm()
assert(
	#selected.entries == 2
		and selected.entries[1].id == "entry-three"
		and selected.entries[1].annotation == "review first"
		and selected.entries[1].pinned
		and selected.entries[2].id == "entry-one",
	"confirmation must preserve pinned annotated ordering"
)
assert(confirmed == selected, "confirmation must route the selected context pack")
assert(context_inspector.close(), "context inspector close must report success")
