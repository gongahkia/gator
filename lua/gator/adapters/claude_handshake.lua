local redact = require("gator.policy.redact")
local M = {}
local Handshake = {}

Handshake.__index = Handshake

local function fail(message)
	error("Gator Claude handshake: " .. redact.text(tostring(message)), 3)
end

local function text(value, name)
	if type(value) ~= "string" or value == "" then
		fail(name .. " must be non-empty text")
	end
	return value
end

local function reason(value, fallback)
	return type(value) == "string" and value ~= "" and redact.text(value) or fallback
end

local function init(value)
	if type(value) ~= "table" or vim.islist(value) then
		fail("native init must be an object")
	end
	if value.type ~= "system" or value.subtype ~= "init" then
		fail("native handshake requires a system init event")
	end
	return { provider = "claude", id = text(value.session_id, "native init.session_id"), owner = "provider" }
end

local function status(value)
	return vim.deepcopy({
		provider = "claude",
		state = value.state,
		reason = value.reason,
		session = value.session,
	})
end

function M.command(executable)
	return {
		text(executable or "claude", "executable"),
		"-p",
		"--input-format",
		"stream-json",
		"--output-format",
		"stream-json",
		"--verbose",
	}
end

function M.new(opts)
	if type(opts) ~= "table" or type(opts.open) ~= "function" then
		fail("new requires an open callback")
	end
	for key in pairs(opts) do
		if key ~= "open" and key ~= "executable" then
			fail("new contains unsupported field: " .. tostring(key))
		end
	end
	return setmetatable({
		open = opts.open,
		command = M.command(opts.executable),
		state = "ready",
	}, Handshake)
end

function M.unavailable(value)
	return setmetatable({ state = "unavailable", reason = reason(value, "Claude transport is unavailable") }, Handshake)
end

function M.is(value)
	return getmetatable(value) == Handshake
end

function Handshake:status()
	if not M.is(self) then
		fail("status requires a Claude handshake")
	end
	return status(self)
end

function Handshake:cancel(value)
	if not M.is(self) then
		fail("cancel requires a Claude handshake")
	end
	if self.state ~= "ready" then
		return false
	end
	self.state, self.reason = "cancelled", reason(value, "Claude handshake cancelled")
	return true
end

function Handshake:connect()
	if not M.is(self) then
		fail("connect requires a Claude handshake")
	end
	if self.state ~= "ready" then
		return status(self)
	end
	local ok, value = pcall(self.open, vim.deepcopy(self.command))
	if not ok then
		self.state, self.reason = "failed", redact.text(tostring(value))
		return status(self)
	end
	local valid, session = pcall(init, value)
	if not valid then
		self.state, self.reason = "failed", redact.text(tostring(session))
		return status(self)
	end
	self.session = session
	return status(self)
end

return M
