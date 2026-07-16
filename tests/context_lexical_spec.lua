local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local lexical = require("gator").module("context").lexical
local root = helpers.tempdir("context-lexical")
local calls = {}
local results = lexical.search({
	cwd = root,
	query = "needle",
	run = function(argv, cwd)
		table.insert(calls, { argv = argv, cwd = cwd })
		if argv[1] == "git" then
			return { code = 0, stdout = root .. "\n" }
		end
		return {
			code = 0,
			stdout = '{"type":"match","data":{"path":{"text":"src/file.lua"},"lines":{"text":"needle()\\n"},"line_number":7}}\n',
		}
	end,
	signal = function(match)
		return { available = true, language = "lua", line = match.line }
	end,
})
assert(
	#results == 1
		and results[1].path == vim.uv.fs_realpath(root) .. "/src/file.lua"
		and results[1].signal.language == "lua",
	"lexical retrieval must preserve Git-rooted ripgrep and Tree-sitter signals"
)
assert(
	calls[1].argv[1] == "git" and calls[2].argv[1] == "rg" and calls[2].argv[2] == "--json",
	"lexical retrieval must use Git and ripgrep protocols"
)
