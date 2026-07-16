local inspect = require("gator").module("context").inspect
local inspector = require("gator.ui").context_inspector
local pack = require("gator").module("context").pack
local submitted
local value = pack.new({
	id = "pack-inspect",
	task_id = "task-inspect",
	entries = {
		{
			id = "entry-inspect",
			kind = "file",
			ref = "README.md",
			provenance = { source = "retrieval", ref = "fixture" },
			trust = "provenance",
			token_estimate = { status = "estimated", tokens = 1 },
			transfer = { eligible = true },
		},
	},
})
assert(
	vim.api.nvim_win_is_valid(inspect.suggest({
		pack = value,
		on_confirm = function(result)
			submitted = result
		end,
	})),
	"inspect mode must open an editable suggestion"
)
assert(not submitted, "inspect mode must not submit before confirmation")
assert(inspector.toggle("entry-inspect"), "inspect mode must require explicit selection")
inspector.confirm()
assert(submitted and submitted.entries[1].id == "entry-inspect", "inspect mode must submit only confirmed context")
assert(inspector.close(), "inspect mode must close after confirmation")
