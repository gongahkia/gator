local retention = require("gator").module("core").retention
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local root = helpers.tempdir("retention")
local paths = {}
local ages = {}
for _, category in ipairs(retention.categories) do
	paths[category] = root .. "/" .. category
	ages[category] = 10
end
helpers.write(paths.transcripts .. "/stale.log", "stale")
helpers.write(paths.transcripts .. "/fresh.log", "fresh")
helpers.write(paths.indices .. "/stale.idx", "stale")
vim.uv.fs_utime(paths.transcripts .. "/stale.log", 1, 1)
vim.uv.fs_utime(paths.transcripts .. "/fresh.log", 95, 95)
vim.uv.fs_utime(paths.indices .. "/stale.idx", 1, 1)

local manager = retention.new({ root = root, paths = paths, max_age = ages })
local plan = manager:plan(100)
assert(#plan == 2, "retention plans must include only stale managed files")
assert(plan[1].path:find("stale", 1, true), "retention plans must be deterministic")
local ok = pcall(manager.prune, manager, plan, false)
assert(not ok, "retention cleanup must require explicit confirmation")
assert(#manager:prune(plan, true) == 2, "confirmed retention cleanup must remove planned files")
assert(vim.fn.filereadable(paths.transcripts .. "/fresh.log") == 1, "retention cleanup must preserve fresh files")
