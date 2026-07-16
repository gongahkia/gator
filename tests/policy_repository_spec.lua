local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local repository = require("gator").module("policy").repository

local root = helpers.tempdir("policy-repository")
helpers.write(root .. "/.gator/repository-policy.json", '{"enabled":true,"rules":{"write_allowed":false}}')
local resolved_root = assert(vim.uv.fs_realpath(root))
local value = repository.load({
	cwd = root,
	run = function(argv, cwd)
		if argv[2] == "rev-parse" then
			return { code = 0, stdout = resolved_root .. "\n" }
		end
		assert(argv[2] == "ls-files" and cwd == resolved_root, "repository policy must verify Git tracking")
		return { code = 0, stdout = ".gator/repository-policy.json\n" }
	end,
})
assert(
	value.available
		and value.policy.scope == "repository"
		and value.policy.target == resolved_root
		and value.policy.rules.write_allowed == false
		and value.policy.provenance.ref == ".gator/repository-policy.json",
	"enabled tracked repository policies must load independently with provenance"
)
local unavailable = repository.load({
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
	"untracked repository policy artifacts must remain unavailable"
)
helpers.write(root .. "/.gator/repository-policy.json", '{"enabled":true,"rules":{"token":"credential"}}')
assert(not pcall(repository.load, {
	cwd = root,
	run = function(argv)
		if argv[2] == "rev-parse" then
			return { code = 0, stdout = resolved_root .. "\n" }
		end
		return { code = 0, stdout = ".gator/repository-policy.json\n" }
	end,
}), "repository policies must reject credentials")
