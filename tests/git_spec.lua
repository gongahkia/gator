local git = require("gator").module("core").git

local calls = 0
local argv = { "git", "status", "--short" }
local boundary = git.new({
	run = function(command, cwd)
		calls = calls + 1
		assert(cwd == "/fixture", "Git boundary must preserve cwd")
		command[2] = "changed"
		return { code = 0, stdout = "clean\n", stderr = "" }
	end,
})
local completed = boundary:run(argv, "/fixture")
assert(
	completed.state == "completed" and completed.code == 0 and completed.stdout == "clean\n" and argv[2] == "status",
	"Git boundary must isolate commands and expose completion"
)

local failed = git.new({
	run = function()
		return { code = 1, stdout = "", stderr = "fatal: token: private-value" }
	end,
}):run({ "git", "status" }, "/fixture")
assert(
	failed.state == "failed" and failed.code == 1 and failed.stderr:find("private%-value") == nil,
	"Git command failures must be explicit and redacted"
)

local unavailable = git.new({
	run = function()
		error("credential: private-value")
	end,
}):run({ "git", "status" }, "/fixture")
assert(
	unavailable.state == "unavailable" and unavailable.stderr:find("private%-value") == nil,
	"Git runner failures must expose unavailable without secrets"
)

local cancelled = boundary:run({ "git", "status" }, "/fixture", {
	cancelled = function()
		return true
	end,
})
assert(cancelled.state == "cancelled" and calls == 1, "Git cancellation must not start a command")
assert(not pcall(boundary.run, boundary, { "status" }, "/fixture"), "Git boundary must reject non-Git commands")
