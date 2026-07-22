local review = require("gator.ui").handoff_review
local adapters = require("gator").module("adapters")

local function contract(context, tool)
	local supported = { available = true, modes = { "native" } }
	return adapters.capabilities.new({
		provider = "claude",
		transport = supported,
		auth = supported,
		session = { available = true, modes = { "create" } },
		permission = supported,
		model = supported,
		command = supported,
		tool = tool,
		context = context,
		usage = supported,
	})
end

local confirmed, cancelled
local window = review.open({
	mode = "manual",
	source_provider = "codex",
	target_provider = "claude",
	content = "handoff token=fixture-secret",
	target_capabilities = contract(
		{ available = true, modes = { "agent_retrieval" } },
		{ available = true, modes = { "native" } }
	),
	opt_in = false,
	on_confirm = function(summary, preflight)
		confirmed = { summary = summary, preflight = preflight }
	end,
	on_cancel = function()
		cancelled = true
	end,
})
assert(vim.api.nvim_win_is_valid(window), "handoff review must create a panel")
local content = table.concat(vim.api.nvim_buf_get_lines(vim.api.nvim_win_get_buf(window), 0, -1, false), "\n")
assert(
	content:find("Target preflight: native", 1, true)
		and not content:find("fixture-secret", 1, true)
		and review.inspect().editable,
	"handoff review must render native target preflight and redacted editable content"
)
assert(review.edit("edited handoff") == "edited handoff", "manual summaries must remain editable before confirmation")
assert(
	review.confirm() and confirmed.summary.content == "edited handoff" and confirmed.preflight.transport == "native",
	"confirmed handoffs must route the reviewed summary and preflight"
)
assert(not review.inspect(), "confirmed handoffs must close the review panel")

review.open({
	mode = "explicit",
	source_provider = "codex",
	target_provider = "claude",
	content = "handoff",
	target_capabilities = contract(
		{ available = true, modes = { "agent_retrieval" } },
		{ available = true, modes = { "native" } }
	),
	opt_in = false,
	on_confirm = function()
		error("token=fixture-secret")
	end,
})
assert(not review.confirm() and review.inspect().state == "failed", "handoff callback failures must remain visible")
content = table.concat(vim.api.nvim_buf_get_lines(0, 0, -1, false), "\n")
assert(not content:find("fixture-secret", 1, true), "handoff callback failures must be redacted")
assert(review.cancel(), "failed handoff panels must close explicitly")

window = review.open({
	mode = "automatic",
	source_provider = "codex",
	target_provider = "claude",
	content = "handoff",
	target_capabilities = contract(
		{ available = true, modes = { "agent_retrieval" } },
		{ available = true, modes = { "native" } }
	),
	opt_in = false,
	on_confirm = function() end,
})
assert(
	review.inspect().state == "unavailable" and not review.inspect().editable,
	"automatic handoffs without opt-in must remain unavailable"
)
assert(review.close(), "unavailable handoff panels must close")
assert(cancelled == nil, "programmatic close must not report cancellation")
review.open({
	mode = "manual",
	source_provider = "codex",
	target_provider = "claude",
	content = "handoff",
	target_capabilities = contract(
		{ available = true, modes = { "agent_retrieval" } },
		{ available = true, modes = { "native" } }
	),
	opt_in = false,
	on_confirm = function() end,
	on_cancel = function()
		cancelled = true
	end,
})
assert(review.cancel() and cancelled, "handoff cancellation must close without transferring content")
assert(not pcall(review.open, {
	mode = "manual",
	source_provider = "codex",
	target_provider = "codex",
	content = "handoff",
	target_capabilities = contract(
		{ available = false, reason = "target unavailable" },
		{ available = true, modes = { "native" } }
	),
	opt_in = false,
	on_confirm = function() end,
}), "handoff review must reject contracts whose provider does not match the target")
