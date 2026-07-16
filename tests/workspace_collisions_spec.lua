local collisions = require("gator").module("workspace").collisions

local value = collisions.detect({
	generated = { "dist/**" },
	worktrees = {
		{ id = "worktree-one", active = true, changed_paths = { "src/shared.lua", "dist/bundle.js" } },
		{ id = "worktree-two", active = true, changed_paths = { "src/shared.lua", "dist/bundle.js" } },
		{ id = "worktree-inactive", active = false, changed_paths = { "src/shared.lua" } },
	},
})
assert(#value.warnings == 2, "active overlaps and generated artifacts must produce warnings")
assert(
	value.warnings[1].kind == "generated"
		and value.warnings[1].path == "dist/bundle.js"
		and table.concat(value.warnings[1].worktree_ids, ",") == "worktree-one,worktree-two",
	"generated artifact warnings must retain active worktree identities"
)
assert(
	value.warnings[2].kind == "overlap"
		and value.warnings[2].path == "src/shared.lua"
		and table.concat(value.warnings[2].worktree_ids, ",") == "worktree-one,worktree-two",
	"overlap warnings must exclude inactive worktrees"
)
assert(
	not pcall(
		collisions.detect,
		{ worktrees = { { id = "worktree-one", active = true, changed_paths = { "../secret" } } } }
	),
	"changed paths must remain worktree-relative"
)
