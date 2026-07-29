local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local references = require("gator.context.references")

local root = helpers.tempdir("context-references")
assert(vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0, "fixture must initialize Git")
helpers.write(root .. "/main.lua", "local answer = 42\n")
helpers.write(root .. "/ignored.txt", "ignored\n")
assert(
	vim.system({ "git", "add", "main.lua" }, { cwd = root, text = true }):wait().code == 0,
	"fixture must track source"
)
helpers.write(root .. "/.gator/references.json", '{"roots":[".agents/skills"]}')
helpers.write(root .. "/.agents/skills/project/SKILL.md", "Project directive\n")
local global = helpers.tempdir("context-reference-global")
helpers.write(global .. "/global/SKILL.md", "Global directive\n")

local settings = { roots = { global }, max_files = 4, max_file_bytes = 4096, max_total_bytes = 8192 }
local candidates = references.candidates({ root = root, settings = settings })
assert(
	candidates.skills[1].label == "#global"
		and candidates.skills[2].label == "#project"
		and candidates.files[1].label == "@main.lua",
	"references must expose configured skills and tracked workspace files only"
)
local resolved = references.resolve({ root = root, settings = settings, prompt = "Inspect #global #project @main.lua" })
assert(
	#resolved.artifacts == 3
		and resolved.context:find("Global directive", 1, true)
		and resolved.context:find("Project directive", 1, true)
		and resolved.context:find("local answer = 42", 1, true),
	"references must resolve selected directive and file content"
)
local unknown = references.resolve({ root = root, settings = settings, prompt = "Inspect #unknown @ignored.txt" })
assert(#unknown.artifacts == 0 and unknown.context == nil, "unknown references must remain literal prompt text")
helpers.write(root .. "/.gator/references.json", '{"roots":["../outside"]}')
assert(
	not pcall(references.candidates, { root = root, settings = settings }),
	"project roots must never escape the Git workspace"
)
