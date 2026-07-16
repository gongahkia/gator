local log = require("gator").module("telemetry").log
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")
local directory = helpers.tempdir("telemetry-log")
local path = directory .. "/diagnostics.log"
local logger = log.open({ path = path, max_bytes = 180, max_files = 2 })
local first = logger:write({
	level = "info",
	task_id = "task-one",
	at = 1,
	message = "Authorization: Bearer token-one",
	fields = { access_key = "access-one", detail = "github_pat_one" },
})
assert(
	first.task_id == "task-one" and first.message:find("token%-one") == nil and first.fields.access_key == "[REDACTED]",
	"diagnostic logs must retain task-safe identifiers and redact credentials"
)
logger:write({ level = "error", task_id = "task-two", at = 2, message = string.rep("x", 180) })
assert(
	vim.fn.filereadable(path .. ".1") == 1 and #logger:paths() == 2,
	"diagnostic logs must rotate within their configured local file limit"
)
local output = table.concat(vim.fn.readfile(path), "\n") .. table.concat(vim.fn.readfile(path .. ".1"), "\n")
assert(
	not output:find("token%-one") and not output:find("access%-one") and not output:find("github_pat_one"),
	"rotated diagnostic logs must only contain redacted values"
)
assert(not pcall(logger.write, logger, {
	level = "info",
	task_id = "task-one",
	at = 3,
	message = "session",
	fields = { session = { provider = "fixture", id = "native", owner = "provider" } },
}), "diagnostic logs must exclude provider-owned sessions")
assert(not pcall(log.open, { path = "relative.log" }), "diagnostic log paths must fail explicitly")
