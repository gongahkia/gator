local diff_review = require("gator.ui").diff_review
local run = require("gator.core.run")
local agent_run = run.new({
	id = "run-review",
	task_id = "task-review",
	provider = { name = "codex", session_id = "native-review" },
	process = { pid = 1, executable = "codex" },
	workspace = { kind = "project", root = "/tmp/gator-review" },
	state = "completed",
	timing = {},
	usage = {},
})
local window = diff_review.open({
	run = agent_run,
	changes = { { path = "lua/gator/init.lua", before = "local old = true\n", after = "local new = true\n" } },
})

assert(vim.api.nvim_win_is_valid(window), "diff review opening must create a window")
assert(diff_review.select(1).path == "lua/gator/init.lua", "diff review selection must preserve changed file paths")
local split = diff_review.open_selected()
assert(vim.wo[split.before].diff and vim.wo[split.after].diff, "diff review must open native diff windows")
assert(
	vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(split.before), 0, -1, false)[1] == "local old = true",
	"diff review must render pre-run content"
)
assert(
	vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(split.after), 0, -1, false)[1] == "local new = true",
	"diff review must render post-run content"
)
assert(diff_review.close(), "diff review close must report success")
assert(
	not vim.api.nvim_win_is_valid(split.before) and not vim.api.nvim_win_is_valid(split.after),
	"review close must clean up diff windows"
)

local ok = pcall(diff_review.open, { run = agent_run, changes = { { path = "file", before = "old" } } })
assert(not ok, "incomplete diff content must fail explicitly")
