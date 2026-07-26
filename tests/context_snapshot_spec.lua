local capture = require("gator.context.capture")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("context-snapshot")
assert(
	vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0,
	"snapshot fixture must initialize Git"
)
helpers.write(root .. "/tracked.lua", "return 'tracked'\n")
helpers.write(root .. "/rename-me.lua", "return 'rename'\n")
assert(
	vim.system({ "git", "add", "tracked.lua", "rename-me.lua" }, { cwd = root, text = true }):wait().code == 0,
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
helpers.write(root .. "/.env.local", "TOKEN=must-not-transfer\n")
helpers.write(root .. "/staged.lua", "return 'staged'\n")
assert(
	vim.system({ "git", "add", "staged.lua" }, { cwd = root, text = true }):wait().code == 0,
	"fixture must stage a new file"
)
assert(
	vim.system({ "git", "mv", "rename-me.lua", "renamed.lua" }, { cwd = root, text = true }):wait().code == 0,
	"fixture must stage rename"
)

local snapshot = capture.snapshot(root, { max_files = 8, max_file_chars = 256 })
local files = {}
for _, file in ipairs(snapshot.files) do
	files[file.path] = file
end
assert(
	files["tracked.lua"].state == "included"
		and files["tracked.lua"].content:find("changed", 1, true)
		and files["tracked.lua"].unstaged
		and files["new.lua"].state == "included"
		and files["staged.lua"].staged
		and files["renamed.lua"].kind == "rename"
		and files["renamed.lua"].rename_from == "rename-me.lua"
		and files["binary.bin"].state == "omitted"
		and files["binary.bin"].reason == "binary",
	"snapshots must preserve staged/unstaged/rename metadata and explicitly omit binary files"
)
assert(
	snapshot.schema_version == 1
		and snapshot.base.head ~= nil
		and type(files["tracked.lua"].content_sha256) == "string"
		and files[".env.local"].state == "omitted"
		and files[".env.local"].reason == "secret-like untracked file",
	"snapshots must persist a base SHA, content hashes, and secret-like untracked exclusions"
)
local markdown = capture.snapshot_markdown(snapshot)
assert(
	markdown:find("Handoff file snapshot", 1, true) and markdown:find("Current source diff", 1, true),
	"snapshot review must expose both files and source diff"
)

local limited = capture.snapshot(root, { max_files = 1, max_file_chars = 1 })
assert(
	#limited.files == 6 and limited.files[1].state == "omitted",
	"snapshot limits must explicitly represent omitted transfer material"
)
