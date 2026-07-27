local M = {
	name = "ui",
	api_version = 2,
	accessibility = require("gator.ui.accessibility"),
	approval_details = require("gator.ui.approval_details"),
	conversation = require("gator.ui.conversation"),
	context_preflight = require("gator.ui.context_preflight"),
	escalation = require("gator.ui.escalation"),
	event_details = require("gator.ui.event_details"),
	glyphs = require("gator.ui.glyphs"),
	markdown = require("gator.ui.markdown"),
	loading = require("gator.ui.loading"),
	picker = require("gator.ui.picker"),
	provider_picker = require("gator.ui.provider_picker"),
	run_graph = require("gator.ui.run_graph"),
	run_events = require("gator.ui.run_events"),
	run_handoff = require("gator.ui.run_handoff"),
	timeline = require("gator.ui.timeline"),
	usage_details = require("gator.ui.usage_details"),
}

function M.close()
	local closed = false
	closed = M.conversation.close() or closed
	closed = M.context_preflight.close() or closed
	closed = M.run_graph.close() or closed
	closed = M.run_events.close() or closed
	closed = M.run_handoff.close() or closed
	closed = M.picker.close() or closed
	closed = M.loading.close() or closed
	return closed
end

function M.set_status(state, status, detail)
	if type(state) ~= "table" or type(state.update) ~= "function" then
		error("Gator UI: set_status requires initialized state", 2)
	end
	state:update({ workspace = detail and { status = status, detail = detail } or { status = status } })
end

return M
