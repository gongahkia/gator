local capture = require("gator.context.capture")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("context-snapshot")
assert(
	vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0,
	"snapshot fixture must initialize Git"
)
helpers.write(root .. "/tracked.lua", "return 'tracked'\n")
assert(
	vim.system({ "git", "add", "tracked.lua" }, { cwd = root, text = true }):wait().code == 0,
	"fixture must stage tracked file"
)
assert(
	vim.system(
		{ "git", "-c", "user.name=Gator", "-c", "user.email=gator@example.invalid", "commit", "-qm", "fixture" },
		{
			cwd = root,
			text = true,
		}
	)
		:wait().code == 0,
	"snapshot fixture must commit baseline"
)
helpers.write(root .. "/tracked.lua", "return 'changed'\n")
helpers.write(root .. "/new.lua", "return 'new'\n")
helpers.write(root .. "/binary.bin", "a\0b")

local snapshot = capture.snapshot(root, { max_files = 8, max_file_chars = 256 })
local files = {}
for _, file in ipairs(snapshot.files) do
	files[file.path] = file
end
assert(
	files["tracked.lua"].state == "included"
		and files["tracked.lua"].content:find("changed", 1, true)
		and files["new.lua"].state == "included"
		and files["binary.bin"].state == "omitted"
		and files["binary.bin"].reason == "binary",
	"snapshots must transfer changed text files and explicitly omit binary files"
)
local markdown = capture.snapshot_markdown(snapshot)
assert(
	markdown:find("Handoff file snapshot", 1, true) and markdown:find("Current source diff", 1, true),
	"snapshot review must expose both files and source diff"
)

local limited = capture.snapshot(root, { max_files = 1, max_file_chars = 1 })
assert(
	#limited.files == 3 and limited.files[1].state == "omitted",
	"snapshot limits must explicitly represent omitted transfer material"
)
