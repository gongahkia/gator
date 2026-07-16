local file = require("gator").module("policy").file
local overlay = require("gator").module("policy").overlay

local defaults = overlay.new({
	scope = "file",
	target = "**/*.lua",
	rules = { write_allowed = false, tools = { read = true, write = false } },
	provenance = { source = "project-policy", ref = ".gator/policy.json" },
})
local generated = overlay.new({
	scope = "file",
	target = "lua/gator/*.lua",
	rules = { tools = { write = true } },
	provenance = { source = "repository-policy", ref = ".gator/repository-policy.json" },
})
local value = file.match({ path = "lua/gator/policy.lua", overlays = { defaults, generated } })
assert(
	#value.matched == 2
		and value.matched[1].pattern == "**/*.lua"
		and value.matched[2].provenance.source == "repository-policy",
	"file policies must retain ordered matching provenance"
)
assert(
	value.rules.write_allowed == false and value.rules.tools.read and value.rules.tools.write,
	"later matching policies must merge and override matching rule fields"
)
value.matched[1].rules.write_allowed = true
assert(not defaults.rules.write_allowed, "file policy explanations must not mutate policy overlays")
assert(file.matches("**/*.lua", "init.lua"), "globstar must explicitly match zero path segments")
assert(file.matches("lua/**/policy.lua", "lua/gator/policy.lua"), "globstar must match complete path segments")
assert(not file.matches("*.lua", "lua/policy.lua"), "single-star must not cross path separators")
assert(not pcall(file.matches, "lua/***/policy.lua", "lua/gator/policy.lua"), "globstar syntax must be unambiguous")
assert(not pcall(file.matches, "lua/?.lua", "lua/x.lua"), "unsupported glob syntax must fail explicitly")
assert(not pcall(file.match, { path = "../outside.lua", overlays = {} }), "path traversal must be rejected")
assert(
	not pcall(file.match, { path = "lua/gator/policy.lua", overlays = { defaults, { scope = "file" } } }),
	"file policy matching must require typed policy overlays"
)
