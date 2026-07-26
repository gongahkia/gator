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
}, root)
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
