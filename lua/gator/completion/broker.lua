local rpc = require("gator.adapters.rpc")
local redact = require("gator.policy.redact")

local M = {}
local Broker = {}
Broker.__index = Broker

local function fail(message)
	error("Gator completion broker: " .. redact.text(tostring(message)), 3)
end

local function now()
	return vim.uv.now()
end

local function default_spawn(argv, root, stdout, stderr, on_exit)
	return vim.system(argv, { cwd = root, text = true, stdout = stdout, stderr = stderr }, on_exit)
end

local function status(sidecar)
	return {
		root = sidecar.root,
		state = sidecar.state,
		pid = sidecar.handle and sidecar.handle.pid or nil,
		failure = sidecar.failure,
		requests = sidecar.requests,
	}
end

local function configured(settings)
	return settings.enabled and #settings.sidecar.argv > 0
end

function M.new(opts)
	opts = opts or {}
	if type(opts) ~= "table" or type(opts.settings) ~= "table" then
		fail("new requires settings")
	end
	if opts.spawn ~= nil and type(opts.spawn) ~= "function" then
		fail("spawn must be a function")
	end
	return setmetatable({ settings = opts.settings, spawn = opts.spawn or default_spawn, sidecars = {}, enabled = nil }, Broker)
end

function Broker:is_enabled()
	if self.enabled == nil then
		return self.settings.enabled
	end
	return self.enabled
end

function Broker:set_enabled(value)
	if type(value) ~= "boolean" then
		fail("enabled must be boolean")
	end
	self.enabled = value
	if not value then
		self:close()
	end
	return self:status()
end

function Broker:start(root)
	local existing = self.sidecars[root]
	if existing and (existing.state == "ready" or existing.state == "starting") then
		return existing
	end
	if existing and existing.next_retry_at and existing.next_retry_at > now() then
		return existing
	end
	local sidecar = existing or { root = root, requests = 0 }
	sidecar.state, sidecar.failure = "starting", nil
	self.sidecars[root] = sidecar
	local function failed(message)
		sidecar.state = "failed"
		sidecar.failure = message
		sidecar.next_retry_at = now() + self.settings.sidecar.restart_backoff_ms
		if sidecar.transport then
			sidecar.transport:fail_all({ code = -32000, message = "completion sidecar unavailable" })
		end
	end
	sidecar.transport = rpc.new({
		write = function(value)
			local ok, result = pcall(sidecar.handle.write, sidecar.handle, value)
			if not ok or result == false then
				failed("sidecar write failed")
			end
		end,
		now = now,
	})
	local ok, handle = pcall(self.spawn, self.settings.sidecar.argv, root, function(_, data)
		if not data or data == "" or sidecar.state == "stopped" then
			return
		end
		local fed, err = pcall(sidecar.transport.feed, sidecar.transport, data)
		if not fed then
			failed(err)
		end
	end, function() end, function(result)
		if sidecar.state ~= "stopped" then
			failed(type(result) == "table" and "sidecar exited" or "sidecar failed")
		end
	end)
	if not ok or type(handle) ~= "table" or type(handle.write) ~= "function" or type(handle.kill) ~= "function" then
		failed("sidecar could not start")
		return sidecar
	end
	sidecar.handle, sidecar.state = handle, "ready"
	return sidecar
end

function Broker:request(params, callback)
	if type(params) ~= "table" or type(params.workspace) ~= "table" or type(params.workspace.root) ~= "string" then
		fail("request requires completion context with a workspace root")
	end
	if type(callback) ~= "function" then
		fail("request requires a callback")
	end
	if not self:is_enabled() then
		callback(nil, { code = "disabled" })
		return nil
	end
	if not configured(self.settings) then
		callback(nil, { code = "unconfigured" })
		return nil
	end
	local sidecar = self:start(params.workspace.root)
	if sidecar.state ~= "ready" then
		callback(nil, { code = "unavailable" })
		return nil
	end
	local request_id = sidecar.transport:request("gator/completion", params, function(result, error)
		if error then
			callback(nil, { code = "sidecar", message = error.message })
			return
		end
		if type(result) ~= "table" or type(result.items) ~= "table" or not vim.islist(result.items) then
			callback(nil, { code = "invalid_response" })
			return
		end
		callback(result.items)
	end, self.settings.sidecar.timeout_ms)
	sidecar.requests = sidecar.requests + 1
	vim.defer_fn(function()
		if sidecar.transport and sidecar.state == "ready" then
			sidecar.transport:expire()
		end
	end, self.settings.sidecar.timeout_ms)
	return { root = sidecar.root, id = request_id }
end

function Broker:cancel(ticket)
	if type(ticket) ~= "table" or type(ticket.root) ~= "string" or type(ticket.id) ~= "number" then
		return false
	end
	local sidecar = self.sidecars[ticket.root]
	if not sidecar or sidecar.state ~= "ready" then
		return false
	end
	local ok = pcall(sidecar.transport.cancel, sidecar.transport, ticket.id)
	return ok
end

function Broker:restart(root)
	if root then
		local sidecar = self.sidecars[root]
		if sidecar and sidecar.handle then
			sidecar.state = "stopped"
			pcall(sidecar.handle.kill, sidecar.handle, 15)
		end
		self.sidecars[root] = nil
		return self:start(root)
	end
	for key in pairs(self.sidecars) do
		self:restart(key)
	end
	return self:status()
end

function Broker:status()
	local sidecars = {}
	for _, sidecar in pairs(self.sidecars) do
		table.insert(sidecars, status(sidecar))
	end
	table.sort(sidecars, function(left, right)
		return left.root < right.root
	end)
	return { enabled = self:is_enabled(), configured = configured(self.settings), sidecars = sidecars }
end

function Broker:close()
	for _, sidecar in pairs(self.sidecars) do
		sidecar.state = "stopped"
		if sidecar.transport then
			sidecar.transport:fail_all({ code = -32800, message = "completion sidecar stopped" })
		end
		if sidecar.handle then
			pcall(sidecar.handle.kill, sidecar.handle, 15)
		end
	end
	self.sidecars = {}
end

return M
