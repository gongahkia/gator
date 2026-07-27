local capture = require("gator.context.capture")
local cleanup = require("gator.workspace.cleanup")
local run_store = require("gator.run_store")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local function git(root, argv, message)
	local result = vim.system(argv, { cwd = root, text = true }):wait()
	assert(result.code == 0, message .. ": " .. (result.stderr or ""))
	return result.stdout or ""
end

local function repository(name, files)
	local root = helpers.tempdir(name)
	git(root, { "git", "init", "-q" }, "fixture must initialize Git")
	for path, content in pairs(files) do
		helpers.write(root .. "/" .. path, content)
	end
	git(root, { "git", "add", "." }, "fixture must stage baseline")
	git(
		root,
		{ "git", "-c", "user.name=Gator", "-c", "user.email=gator@example.invalid", "commit", "-qm", "base" },
		"fixture must commit baseline"
	)
	return root
end

local root = repository("git-edge-snapshot", {
	["rename-source.lua"] = "return 'rename'\n",
	["Case.lua"] = "return 'case'\n",
	["target.lua"] = "return 'target'\n",
})
git(root, { "git", "mv", "rename-source.lua", "renamed.lua" }, "fixture must stage rename")
git(root, { "git", "mv", "Case.lua", "case-stage.lua" }, "fixture must stage case rename")
git(root, { "git", "mv", "case-stage.lua", "case.lua" }, "fixture must complete case-only rename")
helpers.write(
	root .. "/pointer.bin",
	"version https://git-lfs.github.com/spec/v1\noid sha256:" .. string.rep("a", 64) .. "\nsize 42\n"
)
assert(vim.uv.fs_symlink(root .. "/target.lua", root .. "/linked.lua") == true, "fixture must create a symbolic link")
local snapshot = capture.snapshot(root, { max_files = 16, max_file_chars = 4096 })
local files = {}
for _, file in ipairs(snapshot.files) do
	files[file.path] = file
end
assert(
	files["renamed.lua"].kind == "rename"
		and files["renamed.lua"].rename_from == "rename-source.lua"
		and files["case.lua"].kind == "rename"
		and files["case.lua"].rename_from == "Case.lua"
		and files["linked.lua"].state == "omitted"
		and files["linked.lua"].reason == "symbolic link"
		and files["pointer.bin"].state == "omitted"
		and files["pointer.bin"].reason == "Git LFS pointer",
	"snapshot capture must preserve rename/case paths and refuse symlink or LFS-pointer transfer"
)

local sparse = repository(
	"git-edge-sparse",
	{ ["included.lua"] = "return 'included'\n", ["excluded.lua"] = "return 'excluded'\n" }
)
git(sparse, { "git", "sparse-checkout", "init", "--no-cone" }, "fixture must enable sparse checkout")
git(sparse, { "git", "sparse-checkout", "set", "included.lua" }, "fixture must configure sparse checkout")
helpers.write(sparse .. "/included.lua", "return 'changed sparse'\n")
local sparse_snapshot = capture.snapshot(sparse, { max_files = 4, max_file_chars = 4096 })
assert(
	#sparse_snapshot.files == 1
		and sparse_snapshot.files[1].path == "included.lua"
		and sparse_snapshot.files[1].state == "included",
	"snapshot capture must operate on a sparse checkout's present changed paths"
)

local submodule = repository("git-edge-submodule-source", { ["README.md"] = "initial\n" })
local super = repository("git-edge-submodule-super", { ["main.lua"] = "return true\n" })
git(
	super,
	{ "git", "-c", "protocol.file.allow=always", "submodule", "add", submodule, "deps/source" },
	"fixture must add local submodule"
)
git(
	super,
	{ "git", "-c", "user.name=Gator", "-c", "user.email=gator@example.invalid", "commit", "-qm", "submodule" },
	"fixture must commit submodule"
)
helpers.write(super .. "/deps/source/README.md", "changed\n")
git(super, { "git", "-C", super .. "/deps/source", "add", "README.md" }, "fixture must stage submodule change")
git(super, {
	"git",
	"-C",
	super .. "/deps/source",
	"-c",
	"user.name=Gator",
	"-c",
	"user.email=gator@example.invalid",
	"commit",
	"-qm",
	"change",
}, "fixture must commit submodule change")
local submodule_snapshot = capture.snapshot(super, { max_files = 4, max_file_chars = 4096 })
assert(
	#submodule_snapshot.files == 1
		and submodule_snapshot.files[1].path == "deps/source"
		and submodule_snapshot.files[1].state == "omitted"
		and submodule_snapshot.files[1].reason == "submodule",
	"snapshot capture must explicitly omit dirty submodules"
)

local worktree_root = repository("git-edge-worktree", { ["main.lua"] = "return true\n" })
local parent = helpers.tempdir("git-edge-worktree-parent")
local linked = parent .. "/dirty"
git(
	worktree_root,
	{ "git", "worktree", "add", "-qb", "gator/dirty", linked, "HEAD" },
	"fixture must create linked worktree"
)
helpers.write(linked .. "/dirty.lua", "return 'dirty'\n")
local manager = cleanup.new({
	root = worktree_root,
	active = function()
		return false
	end,
	owned = function(path)
		return path == vim.uv.fs_realpath(linked)
	end,
})
assert(#manager:plan() == 0, "cleanup must not select a dirty Gator-owned worktree")
assert(vim.fn.delete(linked .. "/dirty.lua") == 0, "fixture must restore linked worktree")
assert(#manager:plan() == 1, "cleanup may select the same clean inactive owned worktree")

local source = repository("git-edge-handoff-source", { ["file.lua"] = "return 'source'\n" })
local target = repository("git-edge-handoff-target", { ["file.lua"] = "return 'target'\n" })
local source_head = vim.trim(git(source, { "git", "rev-parse", "HEAD" }, "fixture must read source SHA"))
local store = run_store.new(target)
local handoff = {
	base = { head = source_head },
	files = {
		{ path = "file.lua", state = "included", content = "return 'source'\n", apply_content = "return 'source'\n" },
	},
}
assert(
	not pcall(store.materialize_handoff, store, "bundle-conflict", "# bundle", handoff, target),
	"handoff must reject an unresolved target conflict before materializing artifacts"
)
assert(
	helpers.read(target .. "/file.lua") == "return 'target'\n"
		and vim.fn.isdirectory(target .. "/.gator/handoffs/bundle-conflict") == 0,
	"rejected handoff conflicts must leave the target and artifact directory unchanged"
)
local retained = store:materialize_handoff("bundle-conflict", "# bundle", handoff, target, {
	decisions = { ["file.lua"] = "skip" },
})
assert(
	vim.fn.filereadable(retained .. "/files/file.lua") == 1
		and helpers.read(target .. "/file.lua") == "return 'target'\n",
	"explicit conflict recovery must retain the reviewed snapshot without overwriting the target"
)
