local redact = require("gator.policy.redact")

local M = { api_version = 1 }
local Container = {}
Container.__index = Container

local registry = {
	compat = { path = "gator.compat" },
	config = { path = "gator.config" },
	state = { path = "gator.state" },
	motion = { path = "gator.ui.motion" },
	loading = { path = "gator.ui.loading" },
	accessibility = { path = "gator.ui.accessibility" },
	redact = { path = "gator.policy.redact" },
	consent = { path = "gator.telemetry.consent" },
	ui = { path = "gator.ui", module = true },
	adapters = { path = "gator.adapters", module = true },
	context = { path = "gator.context", module = true },
	core = { path = "gator.core", module = true },
	extensions = { path = "gator.extensions", module = true },
	github = { path = "gator.github", module = true },
	indexer = { path = "gator.indexer", module = true },
	performance = { path = "gator.performance", module = true },
	policy = { path = "gator.policy", module = true },
	review = { path = "gator.review", module = true },
	startup = { path = "gator.startup", module = true },
	telemetry = { path = "gator.telemetry", module = true },
	workspace = { path = "gator.workspace", module = true },
}

local function fail(message)
	error("Gator coordinator dependencies: " .. message, 3)
end

local function descriptor(name)
	if type(name) ~= "string" or name == "" then
		fail("dependency name must be a non-empty string")
	end
	local value = registry[name]
	if not value then
		fail("unknown dependency: " .. name)
	end
	return value
end

local function reason(value)
	return redact.text(tostring(value))
end

local function validate(name, value, value_descriptor)
	if type(value) ~= "table" then
		return nil, "dependency returned " .. type(value) .. " instead of a table"
	end
	if not value_descriptor.module then
		return value
	end
	if value.name ~= name then
		return nil, "module did not identify itself as " .. name
	end
	if type(value.api_version) ~= "number" or value.api_version < 1 or value.api_version % 1 ~= 0 then
		return nil, "module did not expose a positive integer API version"
	end
	return value
end

function M.new(opts)
	if opts == nil then
		opts = {}
	end
	if type(opts) ~= "table" then
		fail("new requires an options table")
	end
	for key in pairs(opts) do
		if key ~= "loader" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	local loader = opts.loader or require
	if type(loader) ~= "function" then
		fail("loader must be a function")
	end
	return setmetatable({ cache = {}, loader = loader }, Container)
end

function M.is(value)
	return getmetatable(value) == Container
end

function M.modules()
	local result = {}
	for name, value in pairs(registry) do
		if value.module then
			result[name] = value.path
		end
	end
	return result
end

function Container:status(name)
	if not M.is(self) then
		fail("status requires a dependency container")
	end
	local value_descriptor = descriptor(name)
	local cached = self.cache[name]
	if cached then
		return vim.deepcopy(cached.status)
	end
	local ok, value = pcall(self.loader, value_descriptor.path)
	local service, failure
	if ok then
		service, failure = validate(name, value, value_descriptor)
	else
		failure = reason(value)
	end
	local status
	if service then
		status = { name = name, available = true }
	else
		status = { name = name, available = false, reason = reason(failure) }
	end
	self.cache[name] = { service = service, status = status }
	return vim.deepcopy(status)
end

function Container:require(name)
	local status = self:status(name)
	if not status.available then
		fail("dependency " .. name .. " is unavailable: " .. status.reason)
	end
	return self.cache[name].service
end

function Container:module(name)
	if not M.is(self) then
		fail("module requires a dependency container")
	end
	if not descriptor(name).module then
		fail("dependency is not a public module: " .. name)
	end
	return self:require(name)
end

return M
