local M = { api_version = 1 }
local Store = {}
local Subscription = {}

local function fail(message)
	error("Gator state: " .. message, 3)
end

Store.__index = function(self, key)
	local method = Store[key]
	if method then
		return method
	end
	return self._data[key]
end

Store.__newindex = function(self, key, value)
	if type(key) ~= "string" then
		fail("state field name must be a string")
	end
	if key:sub(1, 1) == "_" then
		rawset(self, key, value)
		return
	end
	self:update({ [key] = value })
end

Subscription.__index = Subscription

local fields = {
	config = true,
	context = true,
	adapters = true,
	workspace = true,
	review = true,
	compatibility = true,
}
local statuses = { ready = true, loading = true, degraded = true, failed = true, recovering = true }

local function object(value, name)
	if type(value) ~= "table" or (vim.islist(value) and next(value) ~= nil) then
		fail(name .. " must be an object")
	end
	return value
end

local function list(value, name)
	if type(value) ~= "table" or not vim.islist(value) then
		fail(name .. " must be an array")
	end
	return value
end

local function config(value)
	object(value, "config")
	if type(value.ui) ~= "table" or type(value.context) ~= "table" then
		fail("config must be resolved Gator settings")
	end
	return value
end

local function workspace(value)
	object(value, "workspace")
	if not statuses[value.status] then
		fail("workspace.status must be ready, loading, degraded, failed, or recovering")
	end
	if value.detail ~= nil and (type(value.detail) ~= "string" or value.detail == "") then
		fail("workspace.detail must be a non-empty string")
	end
	return value
end

local function compatibility(value)
	object(value, "compatibility")
	if type(value.supported) ~= "boolean" then
		fail("compatibility.supported must be boolean")
	end
	return value
end

local function validate(value)
	object(value, "state")
	for key in pairs(value) do
		if not fields[key] then
			fail("state contains unsupported field: " .. tostring(key))
		end
	end
	for key in pairs(fields) do
		if value[key] == nil then
			fail("state requires " .. key)
		end
	end
	config(value.config)
	object(value.context, "context")
	object(value.adapters, "adapters")
	workspace(value.workspace)
	object(value.review, "review")
	compatibility(value.compatibility)
	return value
end

local function changed(before, after)
	local result = {}
	for key in pairs(fields) do
		if not vim.deep_equal(before[key], after[key]) then
			table.insert(result, key)
		end
	end
	table.sort(result)
	return result
end

local function notify(self, changed_fields)
	if self._notifying then
		fail("state cannot change from a subscription callback")
	end
	if #changed_fields == 0 then
		return
	end
	self._version = self._version + 1
	self._notifying = true
	local event = { version = self._version, changed = vim.deepcopy(changed_fields) }
	for id = 1, self._subscription_sequence do
		local callback = self._subscriptions[id]
		if callback then
			local ok, err = xpcall(function()
				callback(self:snapshot(), vim.deepcopy(event))
			end, debug.traceback)
			if not ok then
				self._notifying = false
				fail("subscription " .. id .. " failed: " .. tostring(err))
			end
		end
	end
	self._notifying = false
end

function M.new(settings, compatibility)
	local value = {
		config = settings,
		context = {},
		adapters = {},
		workspace = { status = "ready" },
		review = {},
		compatibility = compatibility,
	}
	validate(value)
	return setmetatable({
		_data = vim.deepcopy(value),
		_notifying = false,
		_subscriptions = {},
		_subscription_sequence = 0,
		_version = 0,
	}, Store)
end

function M.is(value)
	return getmetatable(value) == Store
end

function M.is_subscription(value)
	return getmetatable(value) == Subscription
end

function Store:snapshot()
	if not M.is(self) then
		fail("snapshot requires a state store")
	end
	return vim.deepcopy(self._data)
end

function Store:version()
	if not M.is(self) then
		fail("version requires a state store")
	end
	return self._version
end

function Store:update(patch)
	if not M.is(self) then
		fail("update requires a state store")
	end
	object(patch, "update")
	for key in pairs(patch) do
		if not fields[key] then
			fail("update contains unsupported field: " .. tostring(key))
		end
	end
	local next = self:snapshot()
	for key, value in pairs(patch) do
		next[key] = vim.deepcopy(value)
	end
	validate(next)
	local changed_fields = changed(self._data, next)
	self._data = next
	notify(self, changed_fields)
	return self:snapshot()
end

function Store:mutate(callback)
	if not M.is(self) then
		fail("mutate requires a state store")
	end
	if type(callback) ~= "function" then
		fail("mutate requires a callback")
	end
	local next = self:snapshot()
	local result = callback(next)
	validate(next)
	local changed_fields = changed(self._data, next)
	self._data = next
	notify(self, changed_fields)
	return result
end

function Store:subscribe(callback)
	if not M.is(self) then
		fail("subscribe requires a state store")
	end
	if type(callback) ~= "function" then
		fail("subscribe requires a callback")
	end
	self._subscription_sequence = self._subscription_sequence + 1
	local id = self._subscription_sequence
	self._subscriptions[id] = callback
	return setmetatable({ _id = id, _store = self, _cancelled = false }, Subscription)
end

function Subscription:cancel()
	if not M.is_subscription(self) then
		fail("cancel requires a state subscription")
	end
	if self._cancelled then
		return false
	end
	self._store._subscriptions[self._id] = nil
	self._cancelled = true
	return true
end

return M
