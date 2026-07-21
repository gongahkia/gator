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
	validations = {
		{
			command_id = "unit",
			code = 0,
			passed = true,
			policy = { provenance = { source = "project-policy", ref = ".gator/policy.json" } },
			evidence = { { stream = "stdout", text = "passed token=fixture-secret" } },
		},
	},
})

assert(vim.api.nvim_win_is_valid(window), "diff review opening must create a window")
local content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	content:find("passed · unit · exit 0 · project-policy", 1, true) and not content:find("fixture-secret", 1, true),
	"diff review must render redacted policy-provenanced validation results"
)
assert(diff_review.select(1).path == "lua/gator/init.lua", "diff review selection must preserve changed file paths")
local split = diff_review.open_selected()
assert(vim.wo[split.before].diff and vim.wo[split.after].diff, "diff review must open native diff windows")
assert(vim.api.nvim_get_current_win() == window, "opening a diff must preserve keyboard focus in the review controls")
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
ok = pcall(diff_review.open, {
	run = agent_run,
	validations = { { command_id = "unit", code = 0, passed = true, evidence = {} } },
})
assert(not ok, "diff review must reject validation results without policy provenance")
