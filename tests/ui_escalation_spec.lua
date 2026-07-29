local escalation = require("gator").module("ui").escalation
local overlay = require("gator").module("policy").overlay

local baseline = overlay.new({
	scope = "project",
	target = "/tmp/gator-escalation",
	rules = { write_allowed = false, mode = "plan" },
	provenance = { source = "project-policy", ref = ".gator/policy.json" },
})
local requested = overlay.new({
	scope = "run",
	target = "run-escalation",
	rules = { write_allowed = true, mode = "default" },
	provenance = { source = "run-override", ref = "run-escalation" },
})
local acknowledged
local value = escalation.request({
	provider = "codex",
	baseline = baseline,
	requested = requested,
	on_acknowledge = function(record)
		acknowledged = record
	end,
})
assert(
	value.required and vim.api.nvim_win_is_valid(value.window) and #value.changes == 2,
	"broader modes must open acknowledgement UI"
)
local buffer = vim.api.nvim_win_get_buf(value.window)
assert(
	(vim.bo[buffer].filetype == "gator-text" or vim.bo[buffer].filetype == "gator-escalation")
		and not vim.bo[buffer].modifiable
		and vim.api.nvim_buf_call(buffer, function()
			return vim.fn.maparg("?", "n", false, true).buffer == 1
		end),
	"escalation must provide read-only text output and buffer-local keyboard help"
)
local lines = vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(value.window), 0, -1, false)
assert(
	vim.tbl_contains(lines, "Acknowledgement is required before Gator requests this broader provider mode.")
		and vim.tbl_contains(lines, "  write_allowed: false → true"),
	"escalation UI must visibly explain every broader policy change"
)
escalation.acknowledge()
assert(
	acknowledged
		and acknowledged.provider == "codex"
		and acknowledged.baseline.provenance.source == "project-policy"
		and acknowledged.requested.rules.mode == "default",
	"acknowledgement must retain policy provenance without provider credentials or session changes"
)
local narrower = overlay.new({
	scope = "run",
	target = "run-escalation",
	rules = { write_allowed = false, mode = "read_only" },
	provenance = { source = "run-override", ref = "run-escalation" },
})
assert(not escalation.request({
	provider = "codex",
	baseline = baseline,
	requested = narrower,
	on_acknowledge = function() end,
}).required, "equal or narrower requests must not require escalation acknowledgement")
local unknown = overlay.new({
	scope = "run",
	target = "run-escalation",
	rules = { custom_mode = "broader" },
	provenance = { source = "run-override", ref = "run-escalation" },
})
assert(not pcall(escalation.request, {
	provider = "codex",
	baseline = baseline,
	requested = unknown,
	on_acknowledge = function() end,
}), "unassessable policy changes must fail instead of silently bypassing acknowledgement")
