local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local instructions = require("gator").module("context").instructions
local root = helpers.tempdir("context-instructions")
helpers.write(root .. "/AGENTS.md", "rules")
helpers.write(root .. "/.github/copilot-instructions.md", "copilot")
helpers.write(root .. "/.gator/policy.json", "{}")
local value = instructions.discover({ cwd = root })
assert(
	#value == 3
		and value[1].provenance.ref == "AGENTS.md"
		and value[2].provenance.ref == ".github/copilot-instructions.md",
	"instruction discovery must find project and provider rules"
)
assert(
	value[3].provenance.ref == ".gator/policy.json"
		and value[1].trust == "provenance"
		and not value[1].transfer.eligible,
	"instruction discovery must not elevate trust"
)
