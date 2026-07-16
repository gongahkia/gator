local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local project = require("gator").module("policy").project

local root = helpers.tempdir("policy-project")
helpers.write(root .. "/.gator/policy.json", '{"enabled":true,"rules":{"write_allowed":false}}')
local resolved_root = assert(vim.uv.fs_realpath(root))
local value = project.load({
	cwd = root,
	run = function(argv, cwd)
		if argv[2] == "rev-parse" then
			return { code = 0, stdout = resolved_root .. "\n" }
		end
		assert(argv[2] == "ls-files" and cwd == resolved_root, "project policy must verify Git tracking")
		return { code = 0, stdout = ".gator/policy.json\n" }
	end,
})
assert(
	value.available
		and value.policy.rules.write_allowed == false
		and value.policy.provenance.ref == ".gator/policy.json",
	"enabled tracked project policies must load with provenance"
)
local unavailable = project.load({
	cwd = root,
	run = function(argv)
		if argv[2] == "rev-parse" then
			return { code = 0, stdout = resolved_root .. "\n" }
		end
		return { code = 1, stdout = "" }
	end,
})
assert(
	not unavailable.available and unavailable.reason:find("Git%-tracked"),
	"untracked policy artifacts must remain unavailable"
)
