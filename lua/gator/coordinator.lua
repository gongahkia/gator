local dependencies = require("gator.coordinator.dependencies")
local cancellation = require("gator.coordinator.cancellation")

local M = { name = "coordinator", api_version = 1, inspection_schema_version = 1 }
local Coordinator = {}
local Operation = {}
Coordinator.__index = Coordinator
Operation.__index = Operation
local actions = {
	open = { fields = {} },
	health = { fields = {} },
	export_diagnostics = { fields = {} },
	close = { fields = {} },
	cancel_operation = { fields = { id = true, reason = true } },
	capture_selection = { fields = { target = true, buffer = true, first_line = true, last_line = true } },
	palette = { fields = { id = true } },
}
local action_names =
	{ "open", "health", "export_diagnostics", "close", "cancel_operation", "capture_selection", "palette" }
local operation_kinds = { operation = true, launch = true, handoff = true }

local function fail(message)
	error("Gator coordinator: " .. message, 3)
end

local function configure(container, settings)
	container:require("motion").configure(settings.ui.motion)
	container:require("accessibility").configure(settings.ui)
	container:require("redact").configure({ patterns = settings.telemetry.redaction_patterns })
	container:require("consent").configure({ enabled = settings.telemetry.enabled })
	container:require("context").handoff_summary.configure({
		author = settings.context.handoff.author,
		max_chars = settings.context.handoff.max_chars,
	})
	container:require("context").handoff_pack_review.configure({ review = settings.context.handoff.review })
end

local function options(action, value)
	if value == nil then
		value = {}
	end
	if type(value) ~= "table" then
		fail(action .. " options must be a table")
	end
	for key in pairs(value) do
		if not actions[action].fields[key] then
			fail(action .. " options contain unsupported field: " .. tostring(key))
		end
	end
	return value
end

local function require_string(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be a non-empty string")
	end
	return value
end

local function identifier(value, name)
	value = require_string(value, name)
	if not value:match("^[a-z][a-z0-9_-]*$") then
		fail(name .. " must be a lowercase identifier")
	end
	return value
end

local function operation_key(value, redactor)
	value = require_string(value, "operation key")
	if #value > 255 or not value:match("^[%w][%w_-]*$") then
		fail("operation key must be an opaque identifier up to 255 characters")
	end
	if redactor.text(value) ~= value then
		fail("operation key must not contain sensitive data")
	end
	return value
end

local function operation_kind(value)
	value = value or "operation"
	if type(value) ~= "string" or not operation_kinds[value] then
		fail("operation kind is unavailable: " .. tostring(value))
	end
	return value
end

function M.new(opts)
	local container = dependencies.new()
	local report = container:require("compat").require_supported()
	local settings = container:require("config").resolve(opts)
	configure(container, settings)
	local value = setmetatable({
		_dependencies = container,
		_operations = {},
		_operation_keys = {},
		_startup_recovery = nil,
		_state = container:require("state").new(settings, report),
	}, Coordinator)
	return value
end

function M.is(value)
	return getmetatable(value) == Coordinator
end

function M.is_operation(value)
	return getmetatable(value) == Operation
end

M.modules = dependencies.modules()

function M.is_action(value)
	return type(value) == "string" and actions[value] ~= nil
end

function M.actions()
	return vim.deepcopy(action_names)
end

function M.module(name)
	return dependencies.new():module(name)
end

function Coordinator:state()
	if not M.is(self) or type(self._state) ~= "table" then
		fail("state requires an initialized coordinator")
	end
	return self._state
end

function Coordinator:inspect()
	if not M.is(self) then
		fail("inspect requires an initialized coordinator")
	end
	local state = self:state()
	local operations = {}
	for _, operation in pairs(self._operation_keys) do
		table.insert(operations, operation:status())
	end
	table.sort(operations, function(left, right)
		return left.id == right.id and left.key < right.key or left.id < right.id
	end)
	return {
		schema_version = M.inspection_schema_version,
		state_version = state:version(),
		state = state:snapshot(),
		operations = operations,
	}
end

function Coordinator:open()
	self:bootstrap_recovery()
	return self:dependency("ui").open(self:state())
end

function Coordinator:bootstrap_recovery()
	if not M.is(self) then
		fail("bootstrap_recovery requires an initialized coordinator")
	end
	if not self._startup_recovery then
		local core = self:module("core")
		self._startup_recovery = self:dependency("startup").recover({
			state = self:state(),
			load = function()
				return core.run.open():list()
			end,
		})
	end
	return vim.deepcopy(self._startup_recovery)
end

function Coordinator:health()
	self:state()
	vim.cmd("checkhealth gator")
end

function Coordinator:export_diagnostics()
	if not M.is(self) then
		fail("export_diagnostics requires an initialized coordinator")
	end
	local current = self:state()
	return self:module("core").diagnostic_export.write({
		state = current,
		storage = { sharing = current.config.persistence.sharing },
	})
end

function Coordinator:dispatch(action, opts)
	if not M.is(self) then
		fail("dispatch requires an initialized coordinator")
	end
	if not M.is_action(action) then
		fail("action is unavailable: " .. tostring(action))
	end
	opts = options(action, opts)
	if action == "open" then
		return self:open()
	end
	if action == "health" then
		return self:health()
	end
	if action == "export_diagnostics" then
		return self:export_diagnostics()
	end
	if action == "close" then
		return self:dependency("ui").close()
	end
	if action == "cancel_operation" then
		return self:cancel_operation(opts.id, opts.reason)
	end
	if action == "capture_selection" then
		return self:dependency("ui").selection.capture(self:state(), require_string(opts.target, "target"), {
			buffer = opts.buffer,
			first_line = opts.first_line,
			last_line = opts.last_line,
		})
	end
	return self:dependency("ui").palette.execute(require_string(opts.id, "palette id"))
end

function Coordinator:start_operation(opts)
	if not M.is(self) then
		fail("start_operation requires an initialized coordinator")
	end
	if type(opts) ~= "table" then
		fail("start_operation requires options")
	end
	for key in pairs(opts) do
		if key ~= "id" and key ~= "key" and key ~= "kind" and key ~= "cancel" then
			fail("start_operation contains unsupported field: " .. tostring(key))
		end
	end
	local id = identifier(opts.id, "operation id")
	local key = operation_key(opts.key or id, self:dependency("redact"))
	local kind = operation_kind(opts.kind)
	if opts.cancel ~= nil and type(opts.cancel) ~= "function" then
		fail("operation cancel must be a function")
	end
	local existing = self._operation_keys[key]
	if existing then
		if existing.kind ~= kind then
			fail("operation key is already active for " .. existing.kind)
		end
		return existing
	end
	if self._operations[id] then
		fail("operation is already active: " .. id)
	end
	local value = setmetatable({
		coordinator = self,
		id = id,
		key = key,
		kind = kind,
		token = cancellation.new(),
		active = true,
	}, Operation)
	self._operations[id] = value
	self._operation_keys[key] = value
	if opts.cancel then
		value.token:on_cancel(opts.cancel)
	end
	return value
end

function Coordinator:cancel_operation(id, reason)
	if not M.is(self) then
		fail("cancel_operation requires an initialized coordinator")
	end
	id = require_string(id, "operation id")
	local operation = self._operations[id]
	if not operation then
		fail("operation is unavailable: " .. id)
	end
	return operation.token:cancel(reason)
end

function Coordinator:cancel_all(reason)
	if not M.is(self) then
		fail("cancel_all requires an initialized coordinator")
	end
	local ids = {}
	for id in pairs(self._operations) do
		table.insert(ids, id)
	end
	table.sort(ids)
	local cancelled = 0
	for _, id in ipairs(ids) do
		if self:cancel_operation(id, reason) then
			cancelled = cancelled + 1
		end
	end
	return cancelled
end

function Operation:status()
	if not M.is_operation(self) then
		fail("operation status requires an active operation")
	end
	local token = self.token:status()
	local state = token.cancelled and "cancelled" or (self.active and "running" or "completed")
	return {
		id = self.id,
		key = self.key,
		kind = self.kind,
		state = state,
		active = self.active,
		cancelled = token.cancelled,
		reason = token.reason,
	}
end

function Operation:complete()
	if not M.is_operation(self) then
		fail("operation completion requires an active operation")
	end
	if not self.active then
		return false
	end
	self.coordinator._operations[self.id] = nil
	self.active = false
	return true
end

function Coordinator:dependency(name)
	if not M.is(self) or not dependencies.is(self._dependencies) then
		fail("dependency requires an initialized coordinator")
	end
	return self._dependencies:require(name)
end

function Coordinator:module(name)
	if not M.is(self) or not dependencies.is(self._dependencies) then
		fail("module requires an initialized coordinator")
	end
	return self._dependencies:module(name)
end

return M
