local actions = require("gator.ui.actions")
local health = require("gator.health")
local redact = require("gator.policy.redact")

local M = { api_version = 1 }
local Runtime = {}
Runtime.__index = Runtime

local event_names = {
	["run.created"] = "GatorRunCreated",
	["run.started"] = "GatorRunStarted",
	["run.state_changed"] = "GatorRunStateChanged",
	["run.finished"] = "GatorRunFinished",
	["approval.requested"] = "GatorApprovalRequested",
	["approval.resolved"] = "GatorApprovalResolved",
	["context.prepared"] = "GatorContextPrepared",
	["context.delivered"] = "GatorContextDelivered",
	["handoff.prepared"] = "GatorHandoffPrepared",
	["handoff.reviewed"] = "GatorHandoffReviewed",
	["handoff.delivered"] = "GatorHandoffDelivered",
	["status.changed"] = "GatorStatusChanged",
}

local slots = {
	provider_picker = true,
	run_graph = true,
	context_preflight = true,
	handoff_review = true,
	dashboard = true,
}

local function fail(message)
	error("Gator extensions: " .. redact.text(tostring(message)), 3)
end

local function identifier(value, label)
	if type(value) ~= "string" or not value:match("^[a-z][a-z0-9_-]*$") then
		fail(label .. " must be a lowercase identifier")
	end
	return value
end

local function module_name(value)
	if type(value) ~= "string" or not value:match("^[%a_][%w_.-]*$") then
		fail("module name is invalid")
	end
	return value
end

local function safe(value, path)
	local kind = type(value)
	if kind == "string" then
		return redact.text(value)
	end
	if kind == "number" or kind == "boolean" or kind == "nil" then
		return value
	end
	if kind ~= "table" then
		fail(path .. " must be metadata")
	end
	local result = {}
	if vim.islist(value) then
		for index, item in ipairs(value) do
			result[index] = safe(item, path .. "[" .. index .. "]")
		end
		return result
	end
	for key, item in pairs(value) do
		if type(key) ~= "string" then
			fail(path .. " keys must be strings")
		end
		local lower = key:lower()
		if lower:match("token") or lower:match("secret") or lower:match("credential") or lower:match("password") then
			result[key] = "[REDACTED]"
		else
			result[key] = safe(item, path .. "." .. key)
		end
	end
	return result
end

local function callback(value, label)
	if type(value) ~= "function" then
		fail(label .. " must be a function")
	end
	return value
end

local function extension(value)
	if type(value) == "function" then
		return value
	end
	if type(value) == "table" and type(value.setup) == "function" then
		return value.setup
	end
	fail("module must return a setup function or { setup = function } table")
end

function M.new(opts)
	if type(opts) ~= "table" then
		fail("new requires options")
	end
	for key in pairs(opts) do
		if key ~= "modules" and key ~= "renderers" and key ~= "columns" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	if type(opts.modules) ~= "table" or not vim.islist(opts.modules) then
		fail("modules must be an array")
	end
	local modules, seen = {}, {}
	for index, value in ipairs(opts.modules) do
		value = module_name(value)
		if seen[value] then
			fail("modules contains a duplicate: " .. value)
		end
		seen[value] = true
		modules[index] = value
	end
	return setmetatable({
		modules = modules,
		configured_renderers = vim.deepcopy(opts.renderers or {}),
		configured_columns = vim.deepcopy(opts.columns or {}),
		extensions = {},
		listeners = {},
		providers = {},
		renderers = {},
		columns = {},
		collectors = {},
		redactors = {},
		formatters = {},
		sequence = 0,
	}, Runtime)
end

function Runtime:_record(name)
	local value = self.extensions[name]
	if not value then
		value = { name = name, state = "loading", registrations = {}, notified = false }
		self.extensions[name] = value
	end
	return value
end

function Runtime:_track(name, kind, value)
	table.insert(self:_record(name).registrations, { kind = kind, value = value })
	return value
end

function Runtime:_unregister(name)
	local record = self.extensions[name]
	if not record then
		return
	end
	for index = #record.registrations, 1, -1 do
		local registration = record.registrations[index]
		if registration.kind == "action" then
			actions.unregister(registration.value)
		elseif registration.kind == "health" then
			health.unregister(registration.value)
		elseif registration.kind == "listener" then
			self.listeners[registration.value] = nil
		elseif registration.kind == "provider" then
			self.providers[registration.value] = nil
		elseif registration.kind == "renderer" then
			self.renderers[registration.value] = nil
		elseif registration.kind == "column" then
			self.columns[registration.value] = nil
		elseif registration.kind == "collector" then
			self.collectors[registration.value] = nil
		elseif registration.kind == "redactor" then
			self.redactors[registration.value] = nil
		elseif registration.kind == "formatter" then
			self.formatters[registration.value] = nil
		end
	end
	record.registrations = {}
end

function Runtime:_owner(kind, value)
	for name, record in pairs(self.extensions) do
		for _, registration in ipairs(record.registrations) do
			if registration.kind == kind and registration.value == value then
				return name
			end
		end
	end
	return nil
end

function Runtime:disable(name, reason, quiet)
	local record = self:_record(name)
	if record.state == "disabled" then
		return false
	end
	self:_unregister(name)
	if record.cleanup then
		pcall(record.cleanup)
	end
	record.state = "disabled"
	record.reason = vim.trim((redact.text(tostring(reason)):match("^[^\n]+") or "extension callback failed"))
	if not quiet and not record.notified then
		record.notified = true
		vim.notify("Gator extension " .. name .. " disabled: " .. record.reason, vim.log.levels.ERROR)
	end
	return true
end

function Runtime:_api(name)
	local runtime = self
	local function unique(registry, id, label)
		id = identifier(id, label)
		if registry[id] then
			fail(label .. " is already registered: " .. id)
		end
		return id
	end
	return {
		events = {
			on = function(event, handler)
				if not event_names[event] then
					fail("event is unavailable: " .. tostring(event))
				end
				callback(handler, "event handler")
				runtime.sequence = runtime.sequence + 1
				local id = "listener-" .. runtime.sequence
				runtime.listeners[id] = { extension = name, event = event, handler = handler }
				runtime:_track(name, "listener", id)
				return function()
					if runtime.listeners[id] then
						runtime.listeners[id] = nil
						return true
					end
					return false
				end
			end,
		},
		actions = {
			register = function(value)
				local id = actions.register(value)
				runtime:_track(name, "action", id)
				return id
			end,
		},
		health = {
			register = function(check, handler)
				check = "extension." .. name .. "." .. identifier(check, "health check")
				health.register(check, callback(handler, "health callback"))
				runtime:_track(name, "health", check)
				return check
			end,
		},
		providers = {
			register = function(value)
				if type(value) ~= "table" then
					fail("provider must be a table")
				end
				for key in pairs(value) do
					if
						key ~= "name"
						and key ~= "kind"
						and key ~= "probe"
						and key ~= "start"
						and key ~= "resume"
						and key ~= "argv"
					then
						fail("provider contains unsupported field: " .. tostring(key))
					end
				end
				local id = unique(runtime.providers, value.name, "provider name")
				if value.kind ~= "terminal" and value.kind ~= "acp" then
					fail("provider kind must be terminal or acp")
				end
				callback(value.probe, "provider probe")
				if value.kind == "terminal" then
					callback(value.start, "terminal provider start")
					if value.resume ~= nil then
						callback(value.resume, "terminal provider resume")
					end
				else
					if value.start ~= nil or value.resume ~= nil then
						fail("ACP providers are launched through acp.commands, not custom start/resume callbacks")
					end
					if type(value.argv) ~= "table" or not vim.islist(value.argv) or #value.argv == 0 then
						fail("ACP providers require a non-empty argv command")
					end
					for index, argument in ipairs(value.argv) do
						if type(argument) ~= "string" or argument == "" then
							fail("ACP provider argv[" .. index .. "] must be text")
						end
					end
				end
				local provider = vim.deepcopy(value)
				provider.extension = name
				runtime.providers[id] = provider
				runtime:_track(name, "provider", id)
				return id
			end,
		},
		ui = {
			renderer = function(value)
				if type(value) ~= "table" then
					fail("renderer must be a table")
				end
				for key in pairs(value) do
					if key ~= "id" and key ~= "slot" and key ~= "render" then
						fail("renderer contains unsupported field: " .. tostring(key))
					end
				end
				local id = unique(runtime.renderers, value.id, "renderer id")
				if not slots[value.slot] then
					fail("renderer slot is unavailable")
				end
				runtime.renderers[id] =
					{ slot = value.slot, render = callback(value.render, "renderer callback"), extension = name }
				runtime:_track(name, "renderer", id)
				return id
			end,
			column = function(value)
				if type(value) ~= "table" then
					fail("column must be a table")
				end
				for key in pairs(value) do
					if key ~= "id" and key ~= "label" and key ~= "render" then
						fail("column contains unsupported field: " .. tostring(key))
					end
				end
				local id = unique(runtime.columns, value.id, "column id")
				if type(value.label) ~= "string" or value.label == "" then
					fail("column label must be text")
				end
				runtime.columns[id] =
					{ label = value.label, render = callback(value.render, "column callback"), extension = name }
				runtime:_track(name, "column", id)
				return id
			end,
		},
		context = {
			collector = function(value)
				if type(value) ~= "table" or type(value.name) ~= "string" then
					fail("collector requires name and collect")
				end
				local id = unique(runtime.collectors, value.name, "collector name")
				runtime.collectors[id] = callback(value.collect, "collector callback")
				runtime:_track(name, "collector", id)
				return id
			end,
			redactor = function(value)
				if type(value) ~= "table" or type(value.name) ~= "string" then
					fail("redactor requires name and redact")
				end
				local id = unique(runtime.redactors, value.name, "redactor name")
				runtime.redactors[id] = callback(value.redact, "redactor callback")
				runtime:_track(name, "redactor", id)
				return id
			end,
			handoff_formatter = function(value)
				if type(value) ~= "table" or type(value.name) ~= "string" then
					fail("handoff formatter requires name and format")
				end
				local id = unique(runtime.formatters, value.name, "handoff formatter name")
				runtime.formatters[id] = callback(value.format, "handoff formatter callback")
				runtime:_track(name, "formatter", id)
				return id
			end,
		},
	}
end

function Runtime:load()
	for _, name in ipairs(self.modules) do
		local record = self:_record(name)
		local ok, value = xpcall(function()
			return extension(require(name))(self:_api(name))
		end, debug.traceback)
		if not ok then
			self:disable(name, value)
		elseif value ~= nil and type(value) ~= "function" then
			self:disable(name, "setup must return nil or a cleanup function")
		else
			record.state, record.cleanup = "ready", value
		end
	end
	return self:status()
end

function Runtime:emit(event, payload)
	if not event_names[event] then
		fail("event is unavailable: " .. tostring(event))
	end
	local value = safe(payload or {}, "event payload")
	local failures = {}
	for id, listener in pairs(vim.deepcopy(self.listeners)) do
		local live = self.listeners[id]
		if live and live.event == event then
			local ok, reason = xpcall(function()
				live.handler(vim.deepcopy(value))
			end, debug.traceback)
			if not ok then
				if live.extension then
					failures[live.extension] = reason
				else
					self.listeners[id] = nil
					vim.notify("Gator lifecycle hook removed: " .. redact.text(tostring(reason)), vim.log.levels.ERROR)
				end
			end
		end
	end
	for name, reason in pairs(failures) do
		self:disable(name, reason)
	end
	pcall(vim.api.nvim_exec_autocmds, "User", {
		pattern = event_names[event],
		modeline = false,
		data = vim.deepcopy(value),
	})
	return value
end

function Runtime:subscribe(event, handler)
	if not event_names[event] then
		fail("event is unavailable: " .. tostring(event))
	end
	callback(handler, "event handler")
	self.sequence = self.sequence + 1
	local id = "listener-" .. self.sequence
	self.listeners[id] = { event = event, handler = handler }
	return function()
		if self.listeners[id] then
			self.listeners[id] = nil
			return true
		end
		return false
	end
end

function Runtime:provider(name)
	return self.providers[name] and vim.deepcopy(self.providers[name]) or nil
end

function Runtime:providers_list()
	local result = {}
	for _, name in ipairs(vim.tbl_keys(self.providers)) do
		table.insert(result, vim.deepcopy(self.providers[name]))
	end
	table.sort(result, function(left, right)
		return left.name < right.name
	end)
	return result
end

function Runtime:renderer(slot)
	if not slots[slot] then
		fail("renderer slot is unavailable")
	end
	local id = self.configured_renderers[slot]
	local value = id ~= "native" and self.renderers[id] or nil
	if value and value.slot == slot then
		return value.render
	end
	return nil
end

function Runtime:render(slot, model)
	local id = self.configured_renderers[slot]
	local renderer = id ~= "native" and self.renderers[id] or nil
	if not renderer or renderer.slot ~= slot then
		return false
	end
	local ok, result = xpcall(function()
		return renderer.render(vim.deepcopy(model))
	end, debug.traceback)
	if not ok then
		self:disable(renderer.extension, result)
		return false
	end
	return result ~= false, result
end

function Runtime:columns()
	local result = {}
	for _, id in ipairs(self.configured_columns) do
		local value = self.columns[id]
		if value then
			result[id] = vim.deepcopy(value)
		end
	end
	return result
end

function Runtime:render_column(id, model)
	local column = self.columns[id]
	if not column then
		return nil
	end
	local ok, value = xpcall(function()
		return column.render(vim.deepcopy(model))
	end, debug.traceback)
	if not ok then
		self:disable(column.extension, value)
		return nil
	end
	if type(value) ~= "string" then
		self:disable(column.extension, "column renderer must return text")
		return nil
	end
	return value
end

function Runtime:collect(context)
	local artifacts = {}
	for name, callback_value in pairs(self.collectors) do
		local ok, value = xpcall(function()
			return callback_value(vim.deepcopy(context))
		end, debug.traceback)
		if ok and type(value) == "table" and type(value.text) == "string" then
			local metadata_ok, metadata = pcall(safe, value.metadata or {}, "collector metadata")
			if metadata_ok then
				artifacts[#artifacts + 1] = { name = name, text = value.text, metadata = metadata }
			else
				local owner = self:_owner("collector", name)
				if owner then
					self:disable(owner, metadata)
				end
			end
		else
			local owner = self:_owner("collector", name)
			if owner then
				self:disable(owner, ok and "collector must return { text = string }" or value)
			end
		end
	end
	return artifacts
end

function Runtime:redact(text)
	if type(text) ~= "string" then
		fail("context text must be text")
	end
	local inspected = redact.inspect(text)
	local value, matches = inspected.text, inspected.matches
	for name, callback_value in pairs(self.redactors) do
		local ok, next_value = xpcall(function()
			return callback_value(value)
		end, debug.traceback)
		if ok and type(next_value) == "string" then
			value = next_value
		else
			local owner = self:_owner("redactor", name)
			if owner then
				self:disable(owner, ok and "redactor must return text" or next_value)
			end
		end
	end
	local final = redact.inspect(value)
	return final.text, matches + final.matches
end

function Runtime:format_handoff(context)
	local sections = {}
	for name, callback_value in pairs(self.formatters) do
		local ok, value = xpcall(function()
			return callback_value(vim.deepcopy(context))
		end, debug.traceback)
		if ok and type(value) == "string" and vim.trim(value) ~= "" then
			local text = self:redact(value)
			table.insert(sections, { name = name, text = text })
		elseif not ok then
			local owner = self:_owner("formatter", name)
			if owner then
				self:disable(owner, value)
			end
		end
	end
	return sections
end

function Runtime:status()
	local result = {}
	for _, name in ipairs(vim.tbl_keys(self.extensions)) do
		local value = self.extensions[name]
		table.insert(result, { name = name, state = value.state, reason = value.reason })
	end
	table.sort(result, function(left, right)
		return left.name < right.name
	end)
	return result
end

function Runtime:close()
	for _, name in ipairs(vim.tbl_keys(self.extensions)) do
		self:disable(name, "Gator reconfigured", true)
	end
	return true
end

return M
