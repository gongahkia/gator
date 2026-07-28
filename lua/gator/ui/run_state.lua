local redact = require("gator.policy.redact")

local M = {}
local states = {
	starting = { label = "Starting", detail = "opening the provider session" },
	running = { label = "Working", detail = "the current turn is active; c cancels and q detaches" },
	waiting_input = { label = "Ready for input", detail = "the provider turn is complete; i sends a follow-up" },
	detached = { label = "Detached", detail = "the chat was closed; use :GatorRuns to reopen the retained run" },
	completed = { label = "Completed", detail = "the provider session finished" },
	failed = { label = "Failed", detail = "the provider session ended with an error; inspect the chat or :GatorRuns" },
	stopped = { label = "Stopped", detail = "the provider session was stopped" },
}

function M.summary(value)
	local state = states[value]
	if state then
		return state.label .. " (" .. value .. ")"
	end
	return "Unknown (" .. redact.text(tostring(value)) .. ")"
end

function M.detail(value)
	local state = states[value]
	return state and state.detail or "the run state is unavailable"
end

return M
