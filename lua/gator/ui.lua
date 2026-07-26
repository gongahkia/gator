local M = {
	name = "ui",
	api_version = 2,
	accessibility = require("gator.ui.accessibility"),
	approval_details = require("gator.ui.approval_details"),
	conversation = require("gator.ui.conversation"),
	context_inspector = require("gator.ui.context_inspector"),
	glyphs = require("gator.ui.glyphs"),
	picker = require("gator.ui.picker"),
	provider_picker = require("gator.ui.provider_picker"),
	run_graph = require("gator.ui.run_graph"),
	run_handoff = require("gator.ui.run_handoff"),
}

function M.close()
	local closed = false
	closed = M.conversation.close() or closed
	closed = M.run_graph.close() or closed
	closed = M.run_handoff.close() or closed
	closed = M.picker.close() or closed
	return closed
end

function M.set_status(state, status, detail)
	if type(state) ~= "table" or type(state.update) ~= "function" then
		error("Gator UI: set_status requires initialized state", 2)
	end
	state:update({ workspace = detail and { status = status, detail = detail } or { status = status } })
end

return M
