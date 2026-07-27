local run_store = require("gator.run_store")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("run-store-handoff")
assert(
	vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0,
	"handoff fixture must initialize Git"
)
helpers.write(root .. "/src/deleted.lua", "return 'remove'\n")
local store = run_store.new(root)
local path = store:materialize_handoff("bundle-transfer", "# Reviewed bundle", {
	files = {
		{ path = "src/example.lua", state = "included", content = "return 'snapshot'" },
		{ path = "src/deleted.lua", state = "deleted" },
		{ path = "binary.bin", state = "omitted", reason = "binary" },
	},
}, root, { decisions = { ["src/deleted.lua"] = "apply" } })
assert(
	vim.fn.filereadable(path .. "/files/src/example.lua") == 1
		and helpers.read(path .. "/files/src/example.lua") == "return 'snapshot'"
		and helpers.read(root .. "/src/example.lua") == "return 'snapshot'"
		and vim.fn.filereadable(root .. "/src/deleted.lua") == 0
		and vim.fn.filereadable(root .. "/.gator/bundles/bundle-transfer.md") == 1,
	"handoff snapshots must retain reviewed copies beneath .gator and apply them to the target workspace"
)
assert(not pcall(store.materialize_handoff, store, "bundle-invalid", "body", {
	files = { { path = "../outside", state = "included", content = "no" } },
}, root), "handoff materialization must reject traversal paths")

local lease = store:put_worktree_lease({
	run_id = "run-transfer",
	repository_root = root,
	common_git_dir = root .. "/.git",
	worktree_root = root .. "/linked-worktree",
	branch = "gator/run-transfer",
	base = "HEAD",
	state = "active",
	created_at = 1,
	updated_at = 1,
})
assert(
	store:worktree_lease("run-transfer").state == "active"
		and store:list_worktree_leases()[1].common_git_dir == root .. "/.git",
	"project-local worktree leases must retain only Git ownership metadata"
)
lease.state, lease.updated_at = "released", 2
store:put_worktree_lease(lease)
assert(
	store:worktree_lease("run-transfer").state == "released" and store:remove_worktree_lease("run-transfer"),
	"terminal worktree leases must remain inspectable until explicit removal"
)
