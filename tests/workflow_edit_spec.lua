local config = require("gator.config")
local state = require("gator.state")
local workflow = require("gator.workflow")
local helpers = dofile(vim.g.gator_test.root .. "/tests/helpers.lua")

local root = helpers.tempdir("workflow-edit")
assert(vim.system({ "git", "init", "-q" }, { cwd = root, text = true }):wait().code == 0, "fixture must initialize Git")
helpers.write(root .. "/source.lua", "local value = 1\n")
assert(
	vim.system({ "git", "add", "source.lua" }, { cwd = root, text = true }):wait().code == 0,
	"fixture must track source"
)
vim.cmd("enew!")
local buffer = vim.api.nvim_get_current_buf()
vim.api.nvim_buf_set_name(buffer, root .. "/source.lua")
vim.api.nvim_buf_set_lines(buffer, 0, -1, false, { "local value = 1" })

local value = workflow.new({
	state = state.new(config.resolve({ edits = { save = "never" } }), { supported = true }),
	root = root,
	readiness = function()
		return { { provider = "pi", available = true, readiness_state = "user_confirmed" } }
	end,
})
value:put({
	id = "run-edit",
	provider = "pi",
	role = "primary",
	transport = "chat",
	state = "waiting_input",
	workspace = { kind = "project", root = root },
	bundle_id = "bundle-edit",
	objective = "Replace selected source",
	transcript = "available",
	usage = { state = "unknown", context_tokens_estimate = 1 },
	budget = { limit_tokens = 0, action = "warn", state = "unbounded" },
	session = { id = "pi-session", resume_supported = true },
	created_at = 1,
	updated_at = 1,
})

value.pending_edits["run-edit"] = { text = "local value = 1", buffer = buffer }
value:append_transcript(
	"run-edit",
	"assistant",
	"<gator-replacement>local value = 2</gator-replacement><gator-replacement>local value = 3</gator-replacement>"
)
assert(not value:complete_edit("run-edit"), "multiple replacement payloads must be rejected")
assert(
	value:events("run-edit")[#value:events("run-edit")].type == "edit.rejected",
	"malformed edit responses must retain only rejection metadata"
)

value.pending_edits["run-edit"] = { text = "local value = 1", buffer = buffer }
value:append_transcript("run-edit", "assistant", "<gator-replacement>local value = 2</gator-replacement>")
local window = value:complete_edit("run-edit")
assert(vim.api.nvim_win_is_valid(window), "one strict replacement payload must open a preview")
local preview = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	preview:find("%-local value = 1") and preview:find("%+local value = 2"),
	"replacement previews must show the exact selected-text diff before application"
)
vim.api.nvim_win_close(window, true)
