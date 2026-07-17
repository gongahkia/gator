local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local runtimepath = vim.opt.runtimepath:get()

assert(#runtimepath == 2, "test runtimepath must contain only Gator and VIMRUNTIME")
assert(runtimepath[1] == vim.g.gator_test.root, "Gator must precede VIMRUNTIME in test runtimepath")
assert(runtimepath[2] == vim.env.VIMRUNTIME, "VIMRUNTIME must remain available to tests")
assert(vim.o.packpath == "", "test packpath must be isolated")
assert(
	vim.o.swapfile == false and vim.o.directory == vim.g.gator_test.tempdir .. "/swap//",
	"swap state must be isolated"
)
assert(
	vim.env.XDG_CACHE_HOME == vim.g.gator_test.tempdir .. "/cache"
		and vim.env.XDG_CONFIG_HOME == vim.g.gator_test.tempdir .. "/config"
		and vim.env.XDG_DATA_HOME == vim.g.gator_test.tempdir .. "/data"
		and vim.env.XDG_STATE_HOME == vim.g.gator_test.tempdir .. "/state",
	"XDG state must be isolated"
)

local dir = helpers.tempdir("fixture")
local path = dir .. "/nested/sample.txt"
helpers.write(path, "temporary fixture")
assert(helpers.read(path) == "temporary fixture", "fixture helpers must round-trip content")
assert(
	helpers.read(helpers.fixture_path("sample.txt")) == "gator fixture\n",
	"fixture helper must read repository fixtures"
)

local ok = pcall(helpers.fixture_path, "../outside")
assert(not ok, "fixture helper must reject path traversal")
