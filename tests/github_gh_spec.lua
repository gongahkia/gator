local gh = require("gator").module("github").gh

local calls = {}
local value = gh.detect({
	cwd = "/tmp/gator-github",
	run = function(argv, cwd)
		table.insert(calls, { argv = argv, cwd = cwd })
		if argv[2] == "--version" then
			return { code = 0, stdout = "gh version 2.73.0 (2026-01-01)" }
		end
		return { code = 0, stdout = "" }
	end,
})
assert(
	value.available
		and value.version == "2.73.0"
		and value.authenticated
		and value.capabilities.issue_import.available
		and value.capabilities.pull_request_import.available
		and #calls == 4
		and calls[2].argv[2] == "auth"
		and calls[2].cwd == "/tmp/gator-github",
	"authenticated GitHub CLI detection must expose supported import capabilities"
)
local unauthenticated = gh.detect({
	run = function(argv)
		if argv[2] == "--version" then
			return { code = 0, stdout = "gh version 2.73.0" }
		end
		if argv[2] == "auth" then
			return { code = 1, stderr = "not logged in" }
		end
		return { code = 0, stdout = "" }
	end,
})
assert(
	not unauthenticated.authenticated
		and not unauthenticated.capabilities.issue_import.available
		and unauthenticated.capabilities.issue_import.reason == "GitHub CLI authentication is unavailable",
	"unauthenticated GitHub CLIs must expose unavailable capabilities without storing account details"
)
local unavailable = gh.detect({
	run = function()
		return { code = 127, stderr = "command not found" }
	end,
})
assert(
	not unavailable.available and unavailable.reason == "GitHub CLI is unavailable",
	"missing GitHub CLIs must report explicit capability absence"
)
assert(not pcall(gh.detect, {
	run = function()
		return { code = 0, stdout = "unexpected version output" }
	end,
}), "unparseable GitHub CLI versions must fail explicitly")
