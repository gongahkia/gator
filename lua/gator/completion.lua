local broker = require("gator.completion.broker")
local context = require("gator.completion.context")
local virtual_text = require("gator.completion.virtual_text")
local health = require("gator.health")

local M = {}
local active
local cmp_hooked = false

local function unavailable(callback, code)
	callback(nil, { code = code })
	return nil
end

function M.setup(settings)
	if type(settings) ~= "table" or type(settings.completion) ~= "table" then
		error("Gator completion: setup requires resolved settings", 3)
	end
	if active then
		active:close()
	end
	active = broker.new({ settings = settings.completion })
	virtual_text.setup({
		settings = settings.completion,
		request = M.request,
		cancel = function(ticket)
			return active and active:cancel(ticket)
		end,
		accept = function(root, id)
			return active and active:accept(root, id)
		end,
	})
	if settings.completion.ui.cmp.enabled then
		local ok, cmp = pcall(require, "cmp")
		if ok then
			cmp.register_source("gator", require("gator.completion.source"):new())
			if not cmp_hooked then
				cmp_hooked = true
				cmp.event:on("confirm_done", function(event)
					local item = event.entry and event.entry.completion_item
					local data = item and item.data
					if data and data.gator_completion_root and data.gator_completion_id then
						M.accept_candidate(data.gator_completion_root, data.gator_completion_id)
					end
				end)
			end
		end
	end
	health.unregister("completion.sidecar")
	health.register("completion.sidecar", function(reporter)
		local status = M.status()
		if not status.enabled then
			reporter.ok("Inline completion is disabled")
		elseif not status.configured then
			reporter.warn(
				"Inline completion needs completion.sidecar.argv",
				"Configure a local JSON-RPC completion sidecar or disable completion."
			)
		else
			reporter.ok("Inline completion sidecar is configured")
		end
	end)
	return M
end

function M.accept_candidate(root, id)
	return active and active:accept(root, id) or false
end

function M.request(buffer, callback)
	if not active then
		return unavailable(callback, "uninitialized")
	end
	local payload, reason = context.document({ buffer = buffer, settings = active.settings })
	if not payload then
		return unavailable(callback, reason or "unavailable")
	end
	local ticket = active:request(payload, function(items, error)
		callback(items, error, payload)
	end)
	return payload.document, ticket
end

function M.complete()
	return virtual_text.complete()
end

function M.clear()
	return virtual_text.clear()
end

function M.next()
	return virtual_text.next()
end

function M.prev()
	return virtual_text.prev()
end

function M.accept()
	return virtual_text.accept()
end

function M.accept_word()
	return virtual_text.accept_word()
end

function M.accept_line()
	return virtual_text.accept_line()
end

function M.status()
	local status = active and active:status() or { enabled = false, configured = false, sidecars = {} }
	status.suggestion = virtual_text.status()
	return status
end

function M.status_string()
	return virtual_text.status_string()
end

function M.enable()
	return active and active:set_enabled(true) or nil
end

function M.disable()
	virtual_text.clear()
	return active and active:set_enabled(false) or nil
end

function M.toggle()
	if not active then
		return nil
	end
	return active:set_enabled(not active:is_enabled())
end

function M.restart()
	return active and active:restart() or nil
end

function M.close()
	virtual_text.close()
	if active then
		active:close()
		active = nil
	end
	context.reset()
end

return M
